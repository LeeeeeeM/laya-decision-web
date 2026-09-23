package laya

/*
#cgo LDFLAGS: -L${SRCDIR}/../../../third_party/tokenizers -ltokenizers -ldl -lm
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/daulet/tokenizers"
)

type Tokenizer struct {
	backend *tokenizers.Tokenizer
	CLS     string
	SEP     string
	PAD     string
	MASK    string
	CLSID   int
	SEPID   int
	PADID   int
	MASKID  int
}

func LoadTokenizer(dir string) (*Tokenizer, error) {
	path := filepath.Join(dir, "tokenizer.json")
	backend, err := tokenizers.FromFile(path)
	if err != nil {
		return nil, fmt.Errorf("load tokenizer: %w", err)
	}
	cfg, err := readJSONFile(filepath.Join(dir, "tokenizer_config.json"))
	if err != nil {
		backend.Close()
		return nil, err
	}
	t := &Tokenizer{backend: backend}
	for _, item := range []struct {
		key string
		set func(string, int)
	}{
		{"cls_token", func(s string, id int) { t.CLS, t.CLSID = s, id }},
		{"sep_token", func(s string, id int) { t.SEP, t.SEPID = s, id }},
		{"pad_token", func(s string, id int) { t.PAD, t.PADID = s, id }},
		{"mask_token", func(s string, id int) { t.MASK, t.MASKID = s, id }},
	} {
		raw := specialToken(cfg, item.key)
		if raw == "" {
			backend.Close()
			return nil, fmt.Errorf("tokenizer missing %s", item.key)
		}
		ids, _ := backend.Encode(raw, false)
		if len(ids) != 1 {
			backend.Close()
			return nil, fmt.Errorf("tokenizer special token %s did not encode to one id", item.key)
		}
		item.set(raw, int(ids[0]))
	}
	return t, nil
}

func specialToken(cfg map[string]any, key string) string {
	if s, ok := cfg[key].(string); ok {
		return s
	}
	if m, ok := cfg[key].(map[string]any); ok {
		if c, ok := m["content"].(string); ok {
			return c
		}
	}
	return ""
}

func (t *Tokenizer) Close() {
	if t != nil && t.backend != nil {
		_ = t.backend.Close()
		t.backend = nil
	}
}

func (t *Tokenizer) Encode(text string, addSpecial bool) []int {
	ids, _ := t.backend.Encode(text, addSpecial)
	out := make([]int, len(ids))
	for i, v := range ids {
		out[i] = int(v)
	}
	return out
}

func readJSONFile(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}
