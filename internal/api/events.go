package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gitstq/nudge/internal/store"
)

// notifyRequest is the canonical publish payload.
type notifyRequest struct {
	Topic string   `json:"topic"`
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Level string   `json:"level"`
	Tags  []string `json:"tags"`
	URL   string   `json:"url"`
}

func (s *Server) handleNotify(w http.ResponseWriter, r *http.Request) {
	var req notifyRequest
	if r.Body != nil {
		dec := jsonDecoder(r)
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
	}
	p := principalFrom(r.Context())
	if p != nil && p.key != nil && p.key.Topic != "" {
		// A topic-scoped key may only publish to its own topic.
		if req.Topic != "" && req.Topic != p.key.Topic {
			writeError(w, http.StatusForbidden, "this key is bound to topic "+p.key.Topic)
			return
		}
		req.Topic = p.key.Topic
	}
	if req.Title == "" && req.Body == "" {
		writeError(w, http.StatusBadRequest, "title or body required")
		return
	}
	e := &store.Event{
		Topic: req.Topic, Title: req.Title, Body: req.Body,
		Level: store.NormalizeLevel(req.Level), Tags: req.Tags, URL: req.URL, Source: "api",
	}
	saved, err := s.St.AddEvent(e)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	events := s.St.Events(store.EventFilter{
		Topic:  strings.TrimSpace(q.Get("topic")),
		Unread: q.Get("unread") == "1" || q.Get("unread") == "true",
		Limit:  limit,
	})
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

type readRequest struct {
	IDs []string `json:"ids"`
	All bool     `json:"all"`
}

func (s *Server) handleReadEvents(w http.ResponseWriter, r *http.Request) {
	var req readRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !req.All && len(req.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "ids or all=true required")
		return
	}
	if err := s.St.MarkRead(req.All, req.IDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteOne(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.St.DeleteEvents(false, []string{id}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleClear(w http.ResponseWriter, r *http.Request) {
	if err := s.St.DeleteEvents(true, nil); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.St.Stats())
}
