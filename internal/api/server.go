// Package api exposes nudge's HTTP surface: the authenticated JSON API, the
// ntfy-compatible publish endpoint, the SSE live stream and the embedded
// single-page inbox.
package api

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gitstq/nudge/internal/auth"
	"github.com/gitstq/nudge/internal/config"
	"github.com/gitstq/nudge/internal/push"
	"github.com/gitstq/nudge/internal/store"
)

//go:embed all:webassets
var embedded embed.FS

// Server wires dependencies together.
type Server struct {
	St        *store.Store
	Cfg       config.Config
	Vapid     *push.VAPIDKeys
	AdminHash string

	buckets map[string]*slidingWindow
	mu      sync.Mutex
}

// NewServer constructs the HTTP handler. webAssets is injected so tests can
// use the same embedded tree.
func NewServer(st *store.Store, cfg config.Config, vapid *push.VAPIDKeys, adminHash string) *Server {
	return &Server{
		St:        st,
		Cfg:       cfg,
		Vapid:     vapid,
		AdminHash: adminHash,
		buckets:   map[string]*slidingWindow{},
	}
}

// Handler builds the fully wired mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /api/v1/vapid-public", s.handleVAPIDPublic)

	// Publishing (source key or admin).
	mux.HandleFunc("POST /api/v1/notify", s.requirePublish(s.handleNotify))

	// Admin-only API.
	mux.HandleFunc("GET /api/v1/events", s.requireAdmin(s.handleListEvents))
	mux.HandleFunc("POST /api/v1/events/read", s.requireAdmin(s.handleReadEvents))
	mux.HandleFunc("DELETE /api/v1/events/{id}", s.requireAdmin(s.handleDeleteOne))
	mux.HandleFunc("POST /api/v1/events/clear", s.requireAdmin(s.handleClear))
	mux.HandleFunc("GET /api/v1/stats", s.requireAdmin(s.handleStats))
	mux.HandleFunc("GET /api/v1/stream", s.requireAdmin(s.handleStream))

	mux.HandleFunc("GET /api/v1/devices", s.requireAdmin(s.handleListDevices))
	mux.HandleFunc("POST /api/v1/devices", s.requireAdmin(s.handleAddDevice))
	mux.HandleFunc("DELETE /api/v1/devices/{id}", s.requireAdmin(s.handleDeleteDevice))

	mux.HandleFunc("GET /api/v1/keys", s.requireAdmin(s.handleListKeys))
	mux.HandleFunc("POST /api/v1/keys", s.requireAdmin(s.handleCreateKey))
	mux.HandleFunc("DELETE /api/v1/keys/{id}", s.requireAdmin(s.handleDeleteKey))

	mux.HandleFunc("GET /api/v1/channels", s.requireAdmin(s.handleListChannels))
	mux.HandleFunc("POST /api/v1/channels", s.requireAdmin(s.handleAddChannel))
	mux.HandleFunc("DELETE /api/v1/channels/{id}", s.requireAdmin(s.handleDeleteChannel))
	mux.HandleFunc("POST /api/v1/channels/{id}/test", s.requireAdmin(s.handleTestChannel))

	// ntfy.sh-compatible publishing (POST/PUT /<topic>).
	mux.HandleFunc("POST /{topic...}", s.requirePublish(s.handleNtfyCompat))
	mux.HandleFunc("PUT /{topic...}", s.requirePublish(s.handleNtfyCompat))

	// Embedded PWA / static assets.
	sub, err := fs.Sub(embedded, "webassets")
	if err != nil {
		log.Fatalf("web assets: %v", err)
	}
	mux.Handle("GET /", http.FileServer(http.FS(sub)))

	return s.recoverAll(s.rateLimit(http.MaxBytesHandler(mux, s.Cfg.MaxBodyBytes)))
}

// ---- principal / middleware ---------------------------------------------

type principal struct {
	admin bool
	key   *store.PublishKey
}

func (s *Server) bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	// Convenience for shell one-liners.
	if q := r.URL.Query().Get("token"); q != "" {
		return q
	}
	return ""
}

func (s *Server) authenticate(r *http.Request) (*principal, bool) {
	token := s.bearer(r)
	if token == "" {
		return nil, false
	}
	if auth.EqualHash(token, s.AdminHash) {
		return &principal{admin: true}, true
	}
	if k, ok := s.St.FindKeyByHash(auth.HashToken(token)); ok {
		return &principal{key: k}, true
	}
	return nil, false
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.authenticate(r)
		if !ok || !p.admin {
			writeError(w, http.StatusUnauthorized, "missing or invalid admin token")
			return
		}
		next(w, r)
	}
}

func (s *Server) requirePublish(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.authenticate(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing or invalid token")
			return
		}
		next(w, r.WithContext(withPrincipal(r.Context(), p)))
	}
}

func (s *Server) recoverAll(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic on %s %s: %v", r.Method, r.URL.Path, rec)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// slidingWindow is a fixed 60s counter per client.
type slidingWindow struct {
	count int
	reset time.Time
}

func (s *Server) rateLimit(next http.Handler) http.Handler {
	if s.Cfg.RatePerMinute <= 0 {
		return next
	}
	go func() { // periodic GC of stale buckets
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for range t.C {
			s.mu.Lock()
			for k, b := range s.buckets {
				if time.Now().After(b.reset) {
					delete(s.buckets, k)
				}
			}
			s.mu.Unlock()
		}
	}()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			ip = strings.TrimSpace(strings.Split(fwd, ",")[0])
		}
		now := time.Now()
		s.mu.Lock()
		b := s.buckets[ip]
		if b == nil || now.After(b.reset) {
			b = &slidingWindow{reset: now.Add(time.Minute)}
			s.buckets[ip] = b
		}
		b.count++
		allowed := b.count <= s.Cfg.RatePerMinute
		s.mu.Unlock()
		if !allowed {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- small helpers --------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("unexpected trailing JSON value")
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleVAPIDPublic(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"public_key": s.Vapid.PublicB64()})
}
