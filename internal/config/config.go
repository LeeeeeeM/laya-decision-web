package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const Version = "0.1.0"

type Config struct {
	ListenAddr      string
	CORSOrigins     []string
	StaticDir       string
	BochaJevAPIKey  string
	BochaJevTimeout time.Duration
	BochaJevMaxQPS  float64
	BochaJevMaxFPS  float64
	LayaModelCache  string
	LayaModelID     string
	LayaModelDir    string
	LayaModelRev    string
	HFToken         string
	AllowMock       bool
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddr:      envOr("LISTEN_ADDR", "127.0.0.1:8080"),
		StaticDir:       envOr("STATIC_DIR", "frontend/dist"),
		BochaJevAPIKey:  firstNonEmpty(os.Getenv("BOCHA_JEV_API_KEY"), os.Getenv("BOCHA_SEARCH_API_KEY")),
		BochaJevTimeout: 15 * time.Second,
		BochaJevMaxQPS:  2,
		BochaJevMaxFPS:  2,
		LayaModelCache:  os.Getenv("LAYA_MODEL_CACHE_DIR"),
		LayaModelID:     os.Getenv("LAYA_MODEL_ID"),
		LayaModelDir:    envOr("LAYA_MODEL_DIR", "models/snake"),
		LayaModelRev:    os.Getenv("LAYA_MODEL_REVISION"),
		HFToken:         os.Getenv("HF_TOKEN"),
		AllowMock:       envBool("ALLOW_MOCK_PROVIDER", true),
	}

	origins := envOr("CORS_ORIGINS", "http://127.0.0.1:5173,http://localhost:5173")
	for _, o := range strings.Split(origins, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			cfg.CORSOrigins = append(cfg.CORSOrigins, o)
		}
	}

	if v := os.Getenv("BOCHA_JEV_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("BOCHA_JEV_TIMEOUT: %w", err)
		}
		cfg.BochaJevTimeout = d
	}
	if v := os.Getenv("BOCHA_JEV_MAX_QPS"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 {
			return Config{}, fmt.Errorf("BOCHA_JEV_MAX_QPS must be a positive number")
		}
		cfg.BochaJevMaxQPS = f
	}
	if v := os.Getenv("BOCHA_JEV_MAX_FPS"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 {
			return Config{}, fmt.Errorf("BOCHA_JEV_MAX_FPS must be a positive number")
		}
		cfg.BochaJevMaxFPS = f
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
