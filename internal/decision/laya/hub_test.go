package laya

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundleCompleteAndIsHubRepoID(t *testing.T) {
	dir := t.TempDir()
	if BundleComplete(dir) {
		t.Fatal("empty dir should be incomplete")
	}
	for _, rel := range requiredBundleFiles {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !BundleComplete(dir) {
		t.Fatal("expected complete after writing required files")
	}

	if !IsHubRepoID("aac6fef/laya-multilingual-coreml-ane") {
		t.Fatal("hub id")
	}
	if IsHubRepoID("models/snake") {
		t.Fatal("local relative path must not be hub id")
	}
	if IsHubRepoID("/abs/path") || IsHubRepoID("./models") {
		t.Fatal("paths must not be hub id")
	}
}

func TestResolveBundleDownloadsWhenMissing(t *testing.T) {
	files := map[string]string{
		"coreml_config.json":                                       `{"format":"laya-coreml-ane","format_version":1}`,
		"rl_agent_config.json":                                     `{"max_len":96}`,
		"encoder/config.json":                                      `{"hidden_size":4,"local_attention":2}`,
		"host_weights.safetensors":                                 "weights",
		"tokenizer/tokenizer.json":                                 "{}",
		"tokenizer/tokenizer_config.json":                          "{}",
		"model.mlpackage/Manifest.json":                            "{}",
		"model.mlpackage/Data/com.apple.CoreML/model.mlmodel":      "mlmodel",
		"model.mlpackage/Data/com.apple.CoreML/weights/weight.bin": "bin",
		"README.md": "skip me",
	}
	var tree []hfTreeEntry
	for p, body := range files {
		tree = append(tree, hfTreeEntry{Type: "file", Path: p, Size: int64(len(body))})
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tree/") {
			_ = json.NewEncoder(w).Encode(tree)
			return
		}
		const prefix = "/aac6fef/fake-ane/resolve/main/"
		if !strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}
		rel := strings.TrimPrefix(r.URL.Path, prefix)
		body, ok := files[rel]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	client := &http.Client{Transport: rewriteHost{base: srv.URL}}
	dest := filepath.Join(t.TempDir(), "pkg")
	got, err := ResolveBundle(context.Background(), HubResolveOptions{
		ModelDir:   dest,
		ModelID:    "aac6fef/fake-ane",
		Revision:   "main",
		HTTPClient: client,
		Logf:       func(string, ...any) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !BundleComplete(got) {
		t.Fatalf("downloaded package incomplete at %s", got)
	}
	if _, err := os.Stat(filepath.Join(got, "README.md")); err == nil {
		t.Fatal("README should not be downloaded")
	}
}

type rewriteHost struct {
	base string
}

func (r rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	base, err := url.Parse(r.base)
	if err != nil {
		return nil, err
	}
	clone := req.Clone(req.Context())
	clone.URL.Scheme = base.Scheme
	clone.URL.Host = base.Host
	clone.Host = base.Host
	return http.DefaultTransport.RoundTrip(clone)
}

func TestAllowHubPath(t *testing.T) {
	if !allowHubPath("tokenizer/foo.json") || !allowHubPath("model.mlpackage/Data/x") {
		t.Fatal("expected allow")
	}
	if allowHubPath("README.md") || allowHubPath("validation.json") {
		t.Fatal("expected deny")
	}
}
