package laya

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
)

func TestDecisionIOLogging(t *testing.T) {
	var buf bytes.Buffer
	p := &Provider{logIO: true, logger: log.New(&buf, "", 0)}
	req := decision.Request{
		Provider: decision.ProviderLaya,
		State:    json.RawMessage(`{"message":"hello"}`),
		Questions: map[string]decision.Question{
			"risk": {Type: decision.TypeNoul, Instructions: "is this safe?"},
		},
	}
	p.logDecisionInput(req)
	p.logDecisionOutput(decision.Response{
		Provider: decision.ProviderLaya,
		Model:    "test-model",
		Answers:  map[string]json.RawMessage{"risk": json.RawMessage(`{"type":"noul","noul":0.8}`)},
	}, nil, 12*time.Millisecond)

	logs := buf.String()
	for _, want := range []string{"laya input:", "laya output:", "hello", "test-model", "elapsed_ms"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("logs missing %q: %s", want, logs)
		}
	}
}
