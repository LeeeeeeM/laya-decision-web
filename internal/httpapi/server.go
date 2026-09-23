package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/LeeeeeeM/laya-decision-web/internal/config"
	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
	"github.com/LeeeeeeM/laya-decision-web/internal/decision/jev"
	"github.com/LeeeeeeM/laya-decision-web/internal/session"
)

const maxBody = 256 << 10

type Server struct {
	Cfg     config.Config
	Manager *session.Manager
	Mux     *http.ServeMux
}

func New(cfg config.Config, mgr *session.Manager) *Server {
	s := &Server{Cfg: cfg, Manager: mgr, Mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.withMiddleware(s.Mux)
}

func (s *Server) routes() {
	s.Mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	s.Mux.HandleFunc("GET /api/v1/capabilities", s.handleCapabilities)
	s.Mux.HandleFunc("POST /api/v1/decisions", s.handleDecisions)
	s.Mux.HandleFunc("POST /api/v1/sessions", s.handleCreateSession)
	s.Mux.HandleFunc("GET /api/v1/sessions/{id}/events", s.handleEvents)
	s.Mux.HandleFunc("POST /api/v1/sessions/{id}/controls", s.handleControls)
	s.Mux.HandleFunc("DELETE /api/v1/sessions/{id}", s.handleDeleteSession)

	if info, err := os.Stat(s.Cfg.StaticDir); err == nil && info.IsDir() {
		fileServer := http.FileServer(http.Dir(s.Cfg.StaticDir))
		s.Mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			path := filepath.Join(s.Cfg.StaticDir, filepath.Clean(r.URL.Path))
			if r.URL.Path != "/" {
				if _, err := os.Stat(path); err != nil {
					http.ServeFile(w, r, filepath.Join(s.Cfg.StaticDir, "index.html"))
					return
				}
			}
			fileServer.ServeHTTP(w, r)
		})
	}
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodOptions {
			if !s.allowOrigin(w, r) {
				writeError(w, http.StatusForbidden, "invalid_request", "origin not allowed", 0)
				return
			}
		} else {
			s.writeCORS(w, r)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		s.writeCORS(w, r)
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) originAllowed(origin string) bool {
	if origin == "" {
		return true
	}
	for _, allowed := range s.Cfg.CORSOrigins {
		if origin == allowed {
			return true
		}
	}
	// Allow same-host loopback pages served by this process (LISTEN_ADDR).
	if strings.HasPrefix(origin, "http://127.0.0.1:") || strings.HasPrefix(origin, "http://localhost:") {
		host := strings.TrimPrefix(strings.TrimPrefix(origin, "http://"), "localhost")
		host = strings.TrimPrefix(host, "127.0.0.1")
		listen := s.Cfg.ListenAddr
		if strings.HasPrefix(listen, "127.0.0.1") || strings.HasPrefix(listen, "localhost") || strings.HasPrefix(listen, ":") {
			listenHost := listen
			if i := strings.LastIndex(listen, ":"); i >= 0 {
				listenHost = listen[i:]
			}
			if host == listenHost {
				return true
			}
		}
	}
	return false
}

func (s *Server) allowOrigin(w http.ResponseWriter, r *http.Request) bool {
	_ = w
	return s.originAllowed(r.Header.Get("Origin"))
}

func (s *Server) writeCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if !s.originAllowed(origin) || origin == "" {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Last-Event-ID")
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": config.Version})
}

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Manager.Capabilities())
}

func (s *Server) handleDecisions(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	var req decision.Request
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), 0)
		return
	}
	provider, ok := s.Manager.GetProvider(req.Provider)
	if !ok || !provider.Available() {
		writeError(w, http.StatusServiceUnavailable, "provider_unavailable", "provider is not available", 0)
		return
	}
	resp, err := provider.Decide(r.Context(), req)
	if err != nil {
		mapProviderError(w, err)
		return
	}
	resp.Timing.RequestMS = float64(time.Since(start).Microseconds()) / 1000
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req session.CreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), 0)
		return
	}
	sess, err := s.Manager.Create(req)
	if err != nil {
		msg := err.Error()
		code := "invalid_request"
		status := http.StatusBadRequest
		if strings.Contains(msg, "unavailable") {
			code = "provider_unavailable"
			status = http.StatusServiceUnavailable
		}
		writeError(w, status, code, msg, 0)
		return
	}
	snap := sess.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": sess.ID,
		"status":     snap.Status,
		"snapshot":   snap.Game,
		"events_url": fmt.Sprintf("/api/v1/sessions/%s/events", sess.ID),
	})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.Manager.Get(id)
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
			fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", ev.ID, ev.Type, ev.Data)
			flusher.Flush()
		}
	}
}

func (s *Server) handleControls(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.Manager.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session_not_found", "session not found", 0)
		return
	}
	var req session.ControlRequest
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

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.Manager.Delete(id) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("empty body")
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return fmt.Errorf("request too large")
		}
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string, retryAfter float64) {
	payload := map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
	if retryAfter > 0 {
		payload["error"].(map[string]any)["retry_after_seconds"] = retryAfter
		w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter))
	}
	if status == http.StatusRequestEntityTooLarge {
		code = "request_too_large"
		payload["error"].(map[string]any)["code"] = code
	}
	writeJSON(w, status, payload)
}

func mapProviderError(w http.ResponseWriter, err error) {
	var rate *jev.RateLimitedError
	var auth *jev.AuthError
	var unavail *jev.UnavailableError
	switch {
	case errors.As(err, &rate):
		writeError(w, http.StatusTooManyRequests, "provider_rate_limited", rate.Error(), rate.RetryAfter.Seconds())
	case errors.As(err, &auth):
		writeError(w, http.StatusBadGateway, "provider_auth_failed", auth.Error(), 0)
	case errors.As(err, &unavail):
		writeError(w, http.StatusServiceUnavailable, "provider_unavailable", unavail.Error(), 0)
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "provider_timeout", "provider call timed out", 0)
	default:
		msg := err.Error()
		if strings.Contains(msg, "criteria") || strings.Contains(msg, "questions") || strings.Contains(msg, "state") {
			writeError(w, http.StatusUnprocessableEntity, "model_input_invalid", msg, 0)
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request", msg, 0)
	}
}
