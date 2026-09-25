package config_test

import (
	"os"
	"testing"

	"github.com/LeeeeeeM/laya-decision-web/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("LISTEN_ADDR", "")
	t.Setenv("BOCHA_JEV_API_KEY", "")
	t.Setenv("LAYA_MODEL_DIR", "")
	t.Setenv("LAYA_LOG_IO", "")
	// Clear may not unset if empty string still counts — set explicitly then load.
	_ = os.Unsetenv("LISTEN_ADDR")
	_ = os.Unsetenv("LAYA_MODEL_DIR")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "127.0.0.1:18080" {
		t.Fatalf("listen=%s", cfg.ListenAddr)
	}
	if cfg.LayaModelDir != "models/snake" {
		t.Fatalf("model dir=%s", cfg.LayaModelDir)
	}
	if cfg.BochaJevMaxFPS != 2 {
		t.Fatalf("max fps=%v", cfg.BochaJevMaxFPS)
	}
	if cfg.LayaLogIO {
		t.Fatal("Laya IO logging should be disabled by default")
	}
}

func TestLoadLayaLogIO(t *testing.T) {
	t.Setenv("LAYA_LOG_IO", "true")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.LayaLogIO {
		t.Fatal("expected Laya IO logging to be enabled")
	}
}

func TestLoadRejectsBadQPS(t *testing.T) {
	t.Setenv("BOCHA_JEV_MAX_QPS", "-1")
	defer os.Unsetenv("BOCHA_JEV_MAX_QPS")
	if _, err := config.Load(); err == nil {
		t.Fatal("expected error")
	}
}
