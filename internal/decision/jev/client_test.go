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

func TestJevSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing auth")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{
				"refund": map[string]any{"type": "noul", "noul": 0.9, "confidence": 0.9},
			},
			"usage": map[string]any{"input_tokens": 10, "output_tokens": 0},
		})
	}))
	defer srv.Close()

	c := jev.New("test-key", 5*time.Second, 100)
	c.Endpoint = srv.URL

	state, _ := json.Marshal("hello")
	crit, _ := json.Marshal(map[string]string{"a": "desc"})
	resp, err := c.Decide(context.Background(), decision.Request{
		Provider: decision.ProviderBochaJev,
		State:    state,
		Questions: map[string]decision.Question{
			"refund": {Type: decision.TypeNoul, Instructions: "refund?"},
			"team":   {Type: decision.TypeChoice, Instructions: "team", Criteria: crit},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Answers["refund"] == nil {
		t.Fatal("missing answer")
	}
}

func TestJevRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "12")
		w.WriteHeader(http.StatusTooManyRequests)
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
	rl, ok := err.(*jev.RateLimitedError)
	if !ok {
		t.Fatalf("want RateLimitedError, got %T %v", err, err)
	}
	if rl.RetryAfter < 10*time.Second {
		t.Fatalf("retry after=%v", rl.RetryAfter)
	}
}

func TestJevAuthFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := jev.New("bad", 5*time.Second, 100)
	c.Endpoint = srv.URL
	state, _ := json.Marshal("hello")
	_, err := c.Decide(context.Background(), decision.Request{
		State: state,
		Questions: map[string]decision.Question{
			"refund": {Type: decision.TypeNoul, Instructions: "refund?"},
		},
	})
	if _, ok := err.(*jev.AuthError); !ok {
		t.Fatalf("want AuthError, got %T %v", err, err)
	}
}
