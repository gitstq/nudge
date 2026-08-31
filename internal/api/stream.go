package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// handleStream is a Server-Sent Events feed of newly stored events, with an
// optional ?topic= filter. It works through simple reverse proxies and needs
// no WebSocket upgrade.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	topic := r.URL.Query().Get("topic")
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	events, cancel := s.St.Subscribe()
	defer cancel()

	// Heartbeat keeps idle proxies happy.
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case e, open := <-events:
			if !open {
				return
			}
			if topic != "" && e.Topic != topic {
				continue
			}
			b, err := json.Marshal(e)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", b)
			flusher.Flush()
		}
	}
}
