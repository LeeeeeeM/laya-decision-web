package laya

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
)

// Platform Laya prompts must leave room for HazardSummary under CoreML max_length=96.
// A zero-room action prefix freezes choice logits across every decision.
func TestPlatformQuestionTokenBudget(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "models", "snake")
	cfg, err := loadAgentConfig(dir)
	if err != nil {
		t.Skip(err)
	}
	manifest, err := loadManifest(dir)
	if err != nil {
		t.Skip(err)
	}
	tok, err := LoadTokenizer(filepath.Join(dir, "tokenizer"))
	if err != nil {
		t.Fatal(err)
	}
	maxLen := cfg.MaxLen
	if manifest.Shape.MaxLength > 0 && manifest.Shape.MaxLength < maxLen {
		maxLen = manifest.Shape.MaxLength
	}
	t.Logf("effective maxLen=%d headMax=%d", maxLen, cfg.HeadMaxLen)

	state := `need=NONE go=RIGHT ground=1 pit1@1.5 enemy@4.0 x=4.0`
	questions := map[string]decision.Question{
		"move": {
			Type:         decision.TypeChoice,
			Instructions: "Pick best move.",
			Criteria:     json.RawMessage(`{"RIGHT":"Best. Matches go=RIGHT.","LEFT":"Worse."}`),
		},
		"action": {
			Type:         decision.TypeChoice,
			Instructions: "Pick best action.",
			Criteria:     json.RawMessage(`{"NONE":"Best. Matches need=NONE.","JUMP":"Worse.","CROUCH":"Worse."}`),
		},
	}
	items, err := prepare(tok, state, questions, maxLen, cfg.HeadMaxLen)
	if err != nil {
		t.Fatal(err)
	}
	stLen := len(tok.Encode(state, false))
	for _, it := range items {
		prefix, _ := buildPrefix(tok, it.Q, it.Q.CriteriaList, cfg.HeadMaxLen)
		room := max(0, maxLen-len(prefix)-1)
		kept := min(stLen, room)
		t.Logf("key=%s seqLen=%d prefixLen=%d room=%d stateTok=%d stateKept=%d",
			it.Key, len(it.IDs), len(prefix), room, stLen, kept)
		if room < stLen {
			t.Errorf("key=%s room=%d < stateTok=%d under maxLen=%d (logits would ignore/truncate state)",
				it.Key, room, stLen, maxLen)
		}
	}
}
