//go:build darwin && cgo

package laya_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/LeeeeeeM/laya-decision-web/internal/coreml"
	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
	"github.com/LeeeeeeM/laya-decision-web/internal/decision/laya"
)

func modelDir(t *testing.T) string {
	t.Helper()
	candidates := []string{
		filepath.Join("..", "..", "..", "models", "snake"),
		filepath.Join("models", "snake"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(filepath.Join(c, "coreml_config.json")); err == nil && !st.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	t.Skip("models/snake not present")
	return ""
}

var (
	providerOnce sync.Once
	sharedProv   *laya.Provider
	providerErr  error
)

func sharedProvider(t *testing.T) *laya.Provider {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping Core ML provider load in -short")
	}
	dir := modelDir(t)
	providerOnce.Do(func() {
		// Prefer CPU in tests: ANE on-device specialization can block for minutes
		// waiting on the Neural Engine daemon during go test.
		units := coreml.UnitsCPU
		if os.Getenv("LAYA_TEST_ANE") == "1" {
			units = coreml.UnitsCPUNE
		}
		sharedProv, providerErr = laya.Open(laya.Options{
			ModelDir:     dir,
			ModelID:      "snake",
			ComputeUnits: units,
		})
	})
	if providerErr != nil {
		t.Fatalf("load laya: %v", providerErr)
	}
	if sharedProv == nil || !sharedProv.Available() {
		t.Fatal("laya unavailable")
	}
	return sharedProv
}

func TestTokenizerSpecialTokensAndChinese(t *testing.T) {
	dir := filepath.Join(modelDir(t), "tokenizer")
	tok, err := laya.LoadTokenizer(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer tok.Close()
	if tok.Encode("hello", false)[0] != 25612 {
		t.Fatal("hello parity")
	}
	ids := tok.Encode("客户要求退款", false)
	if len(ids) == 0 {
		t.Fatal("empty chinese tokenization")
	}
	if tok.MASKID == 0 && tok.MASK == "" {
		t.Fatal("mask token missing")
	}
	if tok.CLS == "" || tok.SEP == "" || tok.PAD == "" {
		t.Fatalf("special token strings unset: %+v", tok)
	}
}

func TestLayaNoulDecisionParitySmoke(t *testing.T) {
	p := sharedProvider(t)
	state, _ := json.Marshal("客户要求退还重复扣除的款项。")
	resp, err := p.Decide(context.Background(), decision.Request{
		Provider: decision.ProviderLaya,
		State:    state,
		Questions: map[string]decision.Question{
			"refund": {Type: decision.TypeNoul, Instructions: "客户是否要求退款？"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var ans struct {
		Noul       float64 `json:"noul"`
		Confidence float64 `json:"confidence"`
		Type       string  `json:"type"`
	}
	if err := json.Unmarshal(resp.Answers["refund"], &ans); err != nil {
		t.Fatal(err)
	}
	if ans.Type != "noul" {
		t.Fatalf("%+v", ans)
	}
	// Golden from Python laya-coreml (ANE). CPU path should stay close.
	if abs(ans.Noul-0.9957) > 0.05 {
		t.Fatalf("noul=%v want ~0.9957", ans.Noul)
	}
	if resp.Usage.InputTokens != 42 {
		t.Fatalf("input_tokens=%d want 42", resp.Usage.InputTokens)
	}
}

func TestLayaChoiceDecision(t *testing.T) {
	p := sharedProvider(t)
	state, _ := json.Marshal("Safe route: yes. Food reachable through empty cells: yes.")
	crit := []byte(`{"UP":"Blocked. Collision.","DOWN":"Safe. Best route to food.","LEFT":"Safe. Slower route.","RIGHT":"Unsafe. Traps the snake."}`)
	resp, err := p.Decide(context.Background(), decision.Request{
		State: state,
		Questions: map[string]decision.Question{
			"move": {
				Type:         decision.TypeChoice,
				Instructions: "Choose the best safe move toward food.",
				Criteria:     crit,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var ans struct {
		Choice        string             `json:"choice"`
		Probabilities map[string]float64 `json:"probabilities"`
	}
	if err := json.Unmarshal(resp.Answers["move"], &ans); err != nil {
		t.Fatal(err)
	}
	if len(ans.Probabilities) != 4 || ans.Choice == "" {
		t.Fatalf("%+v", ans)
	}
	sum := 0.0
	for _, v := range ans.Probabilities {
		sum += v
	}
	if abs(sum-1) > 0.02 {
		t.Fatalf("prob sum=%v", sum)
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
