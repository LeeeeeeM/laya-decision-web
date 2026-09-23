package httpapi

import (
	"fmt"
	"net/http"

	"github.com/LeeeeeeM/laya-decision-web/internal/platform"
)

func (s *Server) handlePlatformCreate(w http.ResponseWriter, r *http.Request) {
	if s.Platform == nil {
		writeError(w, http.StatusServiceUnavailable, "invalid_request", "platform unavailable", 0)
		return
	}
	var req platform.CreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), 0)
		return
	}
	sess, err := s.Platform.Create(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), 0)
		return
	}
	snap := sess.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": sess.ID,
		"status":     snap.Status,
		"snapshot":   snap,
		"events_url": fmt.Sprintf("/api/v1/platform/sessions/%s/events", sess.ID),
	})
}

func (s *Server) handlePlatformEvents(w http.ResponseWriter, r *http.Request) {
	if s.Platform == nil {
		writeError(w, http.StatusServiceUnavailable, "invalid_request", "platform unavailable", 0)
		return
	}
	id := r.PathValue("id")
	sess, ok := s.Platform.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session_not_found", "session not found", 0)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "invalid_request", "streaming unsupported", 0)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, unsub := sess.Subscribe(64)
	defer unsub()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", ev.ID, ev.Type, string(ev.Data))
			flusher.Flush()
		}
	}
}

func (s *Server) handlePlatformControls(w http.ResponseWriter, r *http.Request) {
	if s.Platform == nil {
		writeError(w, http.StatusServiceUnavailable, "invalid_request", "platform unavailable", 0)
		return
	}
	id := r.PathValue("id")
	sess, ok := s.Platform.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session_not_found", "session not found", 0)
		return
	}
	var req platform.ControlRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), 0)
		return
	}
	snap, err := sess.Control(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), 0)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": snap.Status, "snapshot": snap})
}

func (s *Server) handlePlatformDelete(w http.ResponseWriter, r *http.Request) {
	if s.Platform == nil {
		writeError(w, http.StatusServiceUnavailable, "invalid_request", "platform unavailable", 0)
		return
	}
	id := r.PathValue("id")
	_ = s.Platform.Delete(id)
	w.WriteHeader(http.StatusNoContent)
}
