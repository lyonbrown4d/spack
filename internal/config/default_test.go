package config_test

import (
	"testing"

	"github.com/lyonbrown4d/spack/internal/config"
)

func TestDefaultConfigUsesProductionSafeDiagnostics(t *testing.T) {
	cfg := config.DefaultConfig()

	if cfg.Debug.Enable {
		t.Fatal("expected debug routes to be disabled by default")
	}
	if cfg.Logger.Level != "info" {
		t.Fatalf("expected default logger level info, got %q", cfg.Logger.Level)
	}
	if cfg.HTTP.LowMemory {
		t.Fatal("expected Fiber low-memory mode to be disabled by default")
	}
}
