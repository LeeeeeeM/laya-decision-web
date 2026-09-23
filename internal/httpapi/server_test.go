package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LeeeeeeM/laya-decision-web/internal/config"
	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
	"github.com/LeeeeeeM/laya-decision-web/internal/decision/mockprovider"
	"github.com/LeeeeeeM/laya-decision-web/internal/httpapi"
	"github.com/LeeeeeeM/laya-decision-web/internal/session"
)

func newTestServer(t *testing.T) *httpapi.Server {
	t.Helper()
	cfg := config.Config{
		ListenAddr:  "127.0.0.1:0",
		CORSOrigins: []string{"http://127.0.0.1:5173"},
		AllowMock:   true,
	}
	mgr := session.NewManager(map[string]decision.Provider{
		decision.ProviderMock: mockprovider.New(),
	}, 2)
	return httpapi.New(cfg, mgr)
}

func TestHealthAndCapabilities(t *testing.T) {
	srv := newTestServer(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("health %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil)
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("capabilities %d %s", rr.Code, rr.Body.String())
	}
	var caps decision.Capabilities
	if err := json.Unmarshal(rr.Body.Bytes(), &caps); err != nil {
		t.Fatal(err)
	}
	if len(caps.Providers) == 0 {
		t.Fatal("empty providers")
	}
}

func TestDecisionsMock(t *testing.T) {
	srv := newTestServer(t)
	body := map[string]any{
		"provider": "mock",
		"state":    "hello",
		"questions": map[string]any{
			"refund": map[string]any{"type": "noul", "instructions": "refund?"},
		},
	}
	raw, _ := json.Marshal(body)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/decisions", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	var resp decision.Response
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Answers["refund"] == nil {
		t.Fatal("missing answer")
	}
}

func TestCreateSessionAndControl(t *testing.T) {
	srv := newTestServer(t)
	raw, _ := json.Marshal(map[string]any{
		"provider": "mock",
		"fps":      30,
		"seed":     7,
		"guarded":  true,
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	var created struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil || created.SessionID == "" {
		t.Fatalf("%s", rr.Body.String())
	}

	ctrl, _ := json.Marshal(map[string]any{"action": "pause"})
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+created.SessionID+"/controls", bytes.NewReader(ctrl))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("control %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/"+created.SessionID, nil)
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete %d", rr.Code)
	}
}

func TestCORSRejectsUnknownOrigin(t *testing.T) {
	srv := newTestServer(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("Origin", "http://evil.example")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("code=%d", rr.Code)
	}
}
