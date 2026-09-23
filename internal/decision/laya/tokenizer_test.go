//go:build darwin && cgo

package laya_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LeeeeeeM/laya-decision-web/internal/decision/laya"
)

func TestTokenizerHelloParity(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "models", "snake", "tokenizer")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("model tokenizer missing")
	}
	tok, err := laya.LoadTokenizer(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer tok.Close()
	ids := tok.Encode("hello", false)
	if len(ids) != 1 || ids[0] != 25612 {
		t.Fatalf("hello ids=%v want [25612]", ids)
	}
	if tok.CLSID == 0 || tok.MASKID == 0 {
		t.Fatalf("special tokens unset: cls=%d mask=%d", tok.CLSID, tok.MASKID)
	}
}
