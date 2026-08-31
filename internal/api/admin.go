package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gitstq/nudge/internal/auth"
	"github.com/gitstq/nudge/internal/push"
	"github.com/gitstq/nudge/internal/store"
)

// ---- devices --------------------------------------------------------------

type deviceRequest struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	P256DH   string `json:"p256dh"`
	Auth     string `json:"auth"`
	// Subscription allows the browser's PushSubscription JSON shape directly.
	Subscription *struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256DH string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	} `json:"subscription"`
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"devices": s.St.Devices()})
}

func (s *Server) handleAddDevice(w http.ResponseWriter, r *http.Request) {
	var req deviceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Subscription != nil {
		req.Endpoint = req.Subscription.Endpoint
		req.P256DH = req.Subscription.Keys.P256DH
		req.Auth = req.Subscription.Keys.Auth
	}
	d := &store.Device{Name: req.Name, Endpoint: req.Endpoint, P256DH: req.P256DH, Auth: req.Auth}
	saved, err := s.St.AddDevice(d)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	if err := s.St.DeleteDevice(r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- publish keys ---------------------------------------------------------

type keyRequest struct {
	Name  string `json:"name"`
	Topic string `json:"topic"`
}

type keyResponse struct {
	*store.PublishKey
	Token string `json:"token,omitempty"` // shown exactly once on creation
}

func (s *Server) handleListKeys(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"keys": s.St.Keys()})
}

func (s *Server) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	var req keyRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	token, err := auth.GenerateToken("nudg_k")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	k := &store.PublishKey{
		Name:   req.Name,
		Topic:  strings.TrimSpace(req.Topic),
		Hash:   auth.HashToken(token),
		Prefix: auth.Prefix(token),
	}
	saved, err := s.St.AddKey(k)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, keyResponse{PublishKey: saved, Token: token})
}

func (s *Server) handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	if err := s.St.DeleteKey(r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- channels -------------------------------------------------------------

type channelRequest struct {
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Target string   `json:"target"`
	Topics []string `json:"topics"`
}

var validChannelTypes = map[string]bool{
	"webhook": true, "discord": true, "slack": true, "telegram": true, "ntfy": true,
}

func (s *Server) handleListChannels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"channels": s.St.Channels()})
}

func (s *Server) handleAddChannel(w http.ResponseWriter, r *http.Request) {
	var req channelRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	req.Target = strings.TrimSpace(req.Target)
	if req.Name == "" || !validChannelTypes[req.Type] || req.Target == "" {
		writeError(w, http.StatusBadRequest, "name, valid type and target required")
		return
	}
	c := &store.Channel{Name: req.Name, Type: req.Type, Target: req.Target, Topics: req.Topics}
	saved, err := s.St.AddChannel(c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	if err := s.St.DeleteChannel(r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleTestChannel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var found *store.Channel
	for _, c := range s.St.Channels() {
		if c.ID == id {
			found = c
		}
	}
	if found == nil {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	client := &http.Client{Timeout: 8 * time.Second}
	status, err := push.TestChannel(client, found)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": status < 300, "status": status})
}
