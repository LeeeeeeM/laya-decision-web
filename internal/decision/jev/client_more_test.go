package jev_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
	"github.com/LeeeeeeM/laya-decision-web/internal/decision/jev"
)

func TestJevInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"answers":"nope"}`))
	}))
	defer srv.Close()
	c := jev.New("test-key", 5*time.Second, 100)
	c.Endpoint = srv.URL
	state, _ := json.Marshal("hello")
	_, err := c.Decide(context.Background(), decision.Request{
		State: state,
		Questions: map[string]decision.Question{
			"refund": {Type: decision.TypeNoul, Instructions: "refund?"},
		},
	})
	if err == nil {
		t.Fatal("expected invalid response error")
	}
}

func TestJevUnavailableWithoutKey(t *testing.T) {
	c := jev.New("", 5*time.Second, 100)
	if c.Available() {
		t.Fatal("expected unavailable")
	}
	state, _ := json.Marshal("hello")
	_, err := c.Decide(context.Background(), decision.Request{
		State: state,
		Questions: map[string]decision.Question{
			"refund": {Type: decision.TypeNoul, Instructions: "refund?"},
		},
	})
	if _, ok := err.(*jev.UnavailableError); !ok {
		t.Fatalf("want UnavailableError, got %T %v", err, err)
	}
}

func TestJevRetriesThenSucceeds(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n < 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{"refund": map[string]any{"type": "noul", "noul": 0.5}},
		})
	}))
	defer srv.Close()
	c := jev.New("test-key", 5*time.Second, 100)
	c.Endpoint = srv.URL
	state, _ := json.Marshal("hello")
	resp, err := c.Decide(context.Background(), decision.Request{
		State: state,
		Questions: map[string]decision.Question{
			"refund": {Type: decision.TypeNoul, Instructions: "refund?"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Answers["refund"] == nil || n < 2 {
		t.Fatalf("n=%d resp=%+v", n, resp)
	}
}
