package laya

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const defaultHubRepo = "aac6fef/laya-multilingual-coreml-ane"

// DefaultHubRepo is the public ANE package used when LAYA_MODEL_ID is empty.
func DefaultHubRepo() string { return defaultHubRepo }

// Required relative paths for an ANE package usable by Open().
var requiredBundleFiles = []string{
	"coreml_config.json",
	"rl_agent_config.json",
	"encoder/config.json",
	"host_weights.safetensors",
	"tokenizer/tokenizer.json",
	"tokenizer/tokenizer_config.json",
	"model.mlpackage/Manifest.json",
	"model.mlpackage/Data/com.apple.CoreML/model.mlmodel",
	"model.mlpackage/Data/com.apple.CoreML/weights/weight.bin",
}

// HubResolveOptions controls local lookup and Hugging Face snapshot download.
type HubResolveOptions struct {
	ModelDir   string // preferred local directory (may be empty / incomplete)
	ModelID    string // Hub repo id, e.g. aac6fef/laya-multilingual-coreml-ane
	Revision   string // Hub revision; empty => main
	CacheDir   string // download root when ModelDir is empty
	HFToken    string
	HTTPClient *http.Client
	Logf       func(format string, args ...any)
}

// BundleComplete reports whether dir looks like a loadable ANE package.
func BundleComplete(dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return false
	}
	for _, rel := range requiredBundleFiles {
		fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil || fi.IsDir() || fi.Size() == 0 {
			return false
		}
	}
	return true
}

// IsHubRepoID reports whether s looks like owner/name (not a filesystem path).
func IsHubRepoID(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, `\:`) {
		return false
	}
	if strings.HasPrefix(s, ".") || strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~") {
		return false
	}
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	// Prefer an existing local directory over a Hub id that happens to match owner/name.
	if st, err := os.Stat(s); err == nil && st.IsDir() {
		return false
	}
	switch parts[0] {
	case "models", "model", "tmp", "var", "usr", "home", "Users", "bin", "frontend", "internal", "third_party":
		return false
	}
	return true
}

// ResolveBundle returns a local directory with a complete ANE package.
// If the preferred path is incomplete and a Hub model id is configured,
// it downloads the snapshot into that path (blocking until finished).
func ResolveBundle(ctx context.Context, opts HubResolveOptions) (string, error) {
	logf := opts.Logf
	if logf == nil {
		logf = log.Printf
	}
	client := opts.HTTPClient
	if client == nil {
		client = DefaultHubHTTPClient()
	}

	explicitDir := strings.TrimSpace(opts.ModelDir)
	if explicitDir != "" {
		abs, err := filepath.Abs(explicitDir)
		if err != nil {
			return "", err
		}
		if BundleComplete(abs) {
			logf("laya model: using local package %s", abs)
			return abs, nil
		}
		// Explicit LAYA_MODEL_DIR wins: do not silently fall back to another tree.
		return downloadInto(ctx, client, opts, abs, logf)
	}

	candidates := []string{"models/snake", "../laya-coreml/models/snake"}
	seen := map[string]bool{}
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		if BundleComplete(abs) {
			logf("laya model: using local package %s", abs)
			return abs, nil
		}
	}

	cache := strings.TrimSpace(opts.CacheDir)
	if cache == "" {
		cache = "models"
	}
	repo := strings.TrimSpace(opts.ModelID)
	if repo == "" {
		repo = defaultHubRepo
	}
	dest, err := filepath.Abs(filepath.Join(cache, strings.ReplaceAll(repo, "/", "--")))
	if err != nil {
		return "", err
	}
	if BundleComplete(dest) {
		logf("laya model: using cached package %s", dest)
		return dest, nil
	}
	return downloadInto(ctx, client, opts, dest, logf)
}

func downloadInto(ctx context.Context, client *http.Client, opts HubResolveOptions, absDest string, logf func(string, ...any)) (string, error) {
	repo := strings.TrimSpace(opts.ModelID)
	if repo == "" {
		repo = defaultHubRepo
	}
	if !IsHubRepoID(repo) {
		return "", fmt.Errorf("no complete local Laya package at %s (and %q is not a Hub repo id)", absDest, repo)
	}
	if BundleComplete(absDest) {
		logf("laya model: using cached package %s", absDest)
		return absDest, nil
	}
	rev := strings.TrimSpace(opts.Revision)
	if rev == "" {
		rev = "main"
	}
	logf("laya model: downloading %s@%s -> %s (blocking until complete)", repo, rev, absDest)
	if err := downloadHubSnapshot(ctx, client, repo, rev, absDest, opts.HFToken, logf); err != nil {
		return "", err
	}
	if !BundleComplete(absDest) {
		return "", fmt.Errorf("download finished but package still incomplete under %s", absDest)
	}
	logf("laya model: download complete at %s", absDest)
	return absDest, nil
}

type hfTreeEntry struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

func downloadHubSnapshot(ctx context.Context, client *http.Client, repo, rev, dest, token string, logf func(string, ...any)) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	files, err := listHubFiles(ctx, client, repo, rev, token)
	if err != nil {
		return err
	}
	var selected []hfTreeEntry
	var total int64
	for _, f := range files {
		if f.Type != "file" {
			continue
		}
		if !allowHubPath(f.Path) {
			continue
		}
		selected = append(selected, f)
		total += f.Size
	}
	if len(selected) == 0 {
		return fmt.Errorf("hub repo %s@%s has no matching model files", repo, rev)
	}
	logf("laya model: %d files, ~%s", len(selected), formatBytes(total))

	var done int64
	for i, f := range selected {
		target := filepath.Join(dest, filepath.FromSlash(f.Path))
		if fi, err := os.Stat(target); err == nil && !fi.IsDir() && fi.Size() > 0 && (f.Size == 0 || fi.Size() == f.Size) {
			done += fi.Size()
			logf("laya model: [%d/%d] skip existing %s", i+1, len(selected), f.Path)
			continue
		}
		logf("laya model: [%d/%d] downloading %s (%s)", i+1, len(selected), f.Path, formatBytes(f.Size))
		if err := downloadHubFile(ctx, client, repo, rev, f.Path, target, token); err != nil {
			return fmt.Errorf("%s: %w", f.Path, err)
		}
		done += f.Size
		logf("laya model: progress ~%s / %s", formatBytes(done), formatBytes(total))
	}
	return nil
}

func allowHubPath(p string) bool {
	p = path.Clean("/" + strings.TrimPrefix(p, "/"))[1:] // normalize separators to /
	switch {
	case p == "coreml_config.json", p == "rl_agent_config.json", p == "host_weights.safetensors":
		return true
	case p == "encoder/config.json":
		return true
	case strings.HasPrefix(p, "tokenizer/"):
		return true
	case strings.HasPrefix(p, "model.mlpackage/"):
		return true
	default:
		return false
	}
}

func listHubFiles(ctx context.Context, client *http.Client, repo, rev, token string) ([]hfTreeEntry, error) {
	var all []hfTreeEntry
	cursor := ""
	for {
		// Do not PathEscape the whole repo id; owner/name must keep the slash.
		u := "https://huggingface.co/api/models/" + repo + "/tree/" + url.PathEscape(rev) + "?recursive=1"
		if cursor != "" {
			u += "&cursor=" + url.QueryEscape(cursor)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		setHFHeaders(req, token)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("list hub tree HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
		}
		var page []hfTreeEntry
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("decode hub tree: %w", err)
		}
		all = append(all, page...)
		cursor = resp.Header.Get("X-Next-Cursor")
		if cursor == "" {
			break
		}
	}
	return all, nil
}

func downloadHubFile(ctx context.Context, client *http.Client, repo, rev, rel, dest, token string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".partial"
	_ = os.Remove(tmp)

	u := "https://huggingface.co/" + repo + "/resolve/" + url.PathEscape(rev) + "/" + escapeHubRel(rel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	setHFHeaders(req, token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(b), 200))
	}
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, dest)
}

func escapeHubRel(rel string) string {
	rel = path.Clean("/" + strings.TrimPrefix(filepath.ToSlash(rel), "/"))
	rel = strings.TrimPrefix(rel, "/")
	parts := strings.Split(rel, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func setHFHeaders(req *http.Request, token string) {
	req.Header.Set("User-Agent", "laya-decision-web/0.1")
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	}
}

func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	f := float64(n)
	units := []string{"KiB", "MiB", "GiB"}
	for _, u := range units {
		f /= 1024
		if f < 1024 {
			return fmt.Sprintf("%.1f %s", f, u)
		}
	}
	return fmt.Sprintf("%.1f TiB", f/1024)
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// DefaultHTTPClient is used by Resolve when none is supplied; exported for tests.
func DefaultHubHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ResponseHeaderTimeout: 120 * time.Second,
			IdleConnTimeout:       90 * time.Second,
		},
	}
}
