package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/LeeeeeeM/laya-decision-web/internal/config"
	"github.com/LeeeeeeM/laya-decision-web/internal/coreml"
	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
	"github.com/LeeeeeeM/laya-decision-web/internal/decision/jev"
	"github.com/LeeeeeeM/laya-decision-web/internal/decision/laya"
	"github.com/LeeeeeeM/laya-decision-web/internal/decision/mockprovider"
	"github.com/LeeeeeeM/laya-decision-web/internal/httpapi"
	"github.com/LeeeeeeM/laya-decision-web/internal/session"
)

func main() {
	loadDotEnv(".env.local")
	loadDotEnv(".env")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	providers := map[string]decision.Provider{}
	jevClient := jev.New(cfg.BochaJevAPIKey, cfg.BochaJevTimeout, cfg.BochaJevMaxQPS)
	providers[decision.ProviderBochaJev] = jevClient
	if cfg.AllowMock {
		providers[decision.ProviderMock] = mockprovider.New()
	}

	layaProvider := loadLaya(cfg)
	providers[decision.ProviderLaya] = layaProvider
	if closer, ok := layaProvider.(interface{ Close() }); ok {
		defer closer.Close()
	}

	mgr := session.NewManager(providers, cfg.BochaJevMaxFPS)
	srv := httpapi.New(cfg, mgr)

	log.Printf("laya-decision-web %s listening on http://%s", config.Version, cfg.ListenAddr)
	if jevClient.Available() {
		log.Printf("provider bocha-jev: available")
	} else {
		log.Printf("provider bocha-jev: unavailable (set BOCHA_JEV_API_KEY)")
	}
	if cfg.AllowMock {
		log.Printf("provider mock: available")
	}
	if layaProvider.Available() {
		log.Printf("provider laya: available")
	} else {
		log.Printf("provider laya: unavailable")
	}

	if err := http.ListenAndServe(cfg.ListenAddr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}

func loadLaya(cfg config.Config) decision.Provider {
	dir := strings.TrimSpace(cfg.LayaModelDir)
	if dir == "" {
		dir = strings.TrimSpace(cfg.LayaModelID)
	}
	if dir == "" {
		for _, candidate := range []string{"models/snake", "../laya-coreml/models/snake"} {
			if st, err := os.Stat(candidate); err == nil && st.IsDir() {
				dir = candidate
				break
			}
		}
	}
	if dir == "" {
		return &unavailableProvider{name: decision.ProviderLaya, reason: "no local Laya model directory configured"}
	}
	if !filepath.IsAbs(dir) {
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
	}
	p, err := laya.Open(laya.Options{
		ModelDir:     dir,
		ModelID:      firstNonEmpty(cfg.LayaModelID, filepath.Base(dir)),
		ComputeUnits: coreml.UnitsCPUNE,
	})
	if err != nil {
		log.Printf("laya load failed: %v", err)
		return &unavailableProvider{name: decision.ProviderLaya, reason: err.Error()}
	}
	return p
}

type unavailableProvider struct {
	name   string
	reason string
}

func (p *unavailableProvider) Name() string    { return p.name }
func (p *unavailableProvider) Available() bool { return false }

func (p *unavailableProvider) Decide(ctx context.Context, req decision.Request) (decision.Response, error) {
	_ = ctx
	_ = req
	return decision.Response{}, fmt.Errorf("%s", p.reason)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}
