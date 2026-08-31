package api

import (
	"io"
	"net/http"
	"strings"

	"github.com/gitstq/nudge/internal/store"
)

// handleNtfyCompat implements the subset of the ntfy.sh publishing API that
// lets existing ntfy clients and plain curl one-liners publish to nudge:
//
//	curl -H "Authorization: Bearer <key>" -H "X-Title: Hi" \
//	     -d "backup finished" http://nudge.local/backups
//
// Query parameters (?title=&priority=&tags=) and the X-* headers are honored.
func (s *Server) handleNtfyCompat(w http.ResponseWriter, r *http.Request) {
	topic := strings.Trim(r.PathValue("topic"), "/")
	if topic == "" || strings.HasPrefix(topic, "api/") {
		writeError(w, http.StatusNotFound, "unknown path")
		return
	}
	q := r.URL.Query()

	body, _ := io.ReadAll(io.LimitReader(r.Body, s.Cfg.MaxBodyBytes))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = q.Get("message")
	}

	title := firstNonEmpty(r.Header.Get("X-Title"), q.Get("title"))
	clickURL := firstNonEmpty(r.Header.Get("X-Click"), q.Get("click"))
	tags := splitCSV(firstNonEmpty(r.Header.Get("X-Tags"), q.Get("tags")))
	level := ntfyPriorityToLevel(firstNonEmpty(r.Header.Get("X-Priority"), q.Get("priority")))

	p := principalFrom(r.Context())
	if p != nil && p.key != nil && p.key.Topic != "" {
		topic = p.key.Topic
	}
	if title == "" && message == "" {
		writeError(w, http.StatusBadRequest, "message required")
		return
	}
	e, err := s.St.AddEvent(&store.Event{
		Topic: topic, Title: title, Body: message, Level: level,
		Tags: tags, URL: clickURL, Source: "ntfy",
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// ntfy-shaped response for client compatibility.
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      e.ID,
		"topic":   e.Topic,
		"title":   e.Title,
		"message": e.Body,
		"tags":    e.Tags,
		"time":    e.CreatedAt.Unix(),
		"event":   "message",
	})
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ntfyPriorityToLevel maps ntfy priorities (1 min … 5 max) to nudge levels.
func ntfyPriorityToLevel(p string) string {
	switch strings.TrimSpace(p) {
	case "5", "4", "max", "high":
		return "error"
	case "3", "default":
		return "info"
	case "2", "low":
		return "success"
	case "1", "min":
		return "success"
	default:
		return "info"
	}
}
