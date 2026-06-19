package config_test

import (
	"testing"
	"time"

	"github.com/lyonbrown4d/spack/internal/config"
)

func TestMemoryCacheParsedTTL(t *testing.T) {
	cfg := config.MemoryCache{TTL: "2m"}
	if got := cfg.ParsedTTL(); got != 2*time.Minute {
		t.Fatalf("expected 2m TTL, got %s", got)
	}

	cfg.TTL = "bad"
	if got := cfg.ParsedTTL(); got != 5*time.Minute {
		t.Fatalf("expected fallback TTL 5m, got %s", got)
	}
}

func TestMemoryCacheEnabled(t *testing.T) {
	cfg := config.MemoryCache{
		Enable:      true,
		Warmup:      true,
		MaxEntries:  128,
		MaxFileSize: 64 * 1024,
		TTL:         "5m",
	}
	if !cfg.Enabled() {
		t.Fatal("expected memory cache to be enabled")
	}

	cfg.MaxEntries = 0
	if cfg.Enabled() {
		t.Fatal("expected memory cache to be disabled when max entries is zero")
	}
}

func TestMemoryCacheMaxCostUsesExplicitByteBudget(t *testing.T) {
	cfg := config.MemoryCache{
		MaxEntries:  128,
		MaxBytes:    1024,
		MaxFileSize: 64,
	}
	if got := cfg.MaxCost(); got != 1024 {
		t.Fatalf("expected explicit max bytes to win, got %d", got)
	}
	if got := cfg.NumCounters(); got != 1280 {
		t.Fatalf("expected counters to be derived from max entries, got %d", got)
	}
}

func TestMemoryCacheMaxCostFallsBackToLegacyEntryBudget(t *testing.T) {
	cfg := config.MemoryCache{
		MaxEntries:  2,
		MaxFileSize: 64,
	}
	if got := cfg.MaxCost(); got != 128 {
		t.Fatalf("expected legacy derived max cost, got %d", got)
	}
}

func TestMemoryCacheWarmupEnabled(t *testing.T) {
	cfg := config.MemoryCache{
		Enable:      true,
		Warmup:      true,
		MaxEntries:  128,
		MaxFileSize: 64 * 1024,
		TTL:         "5m",
	}
	if !cfg.WarmupEnabled() {
		t.Fatal("expected memory cache warmup to be enabled")
	}

	cfg.Warmup = false
	if cfg.WarmupEnabled() {
		t.Fatal("expected memory cache warmup to be disabled")
	}
}

func TestDefaultHTTPServerHeaderHidden(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	if cfg.HTTP.ExposeServerHeader {
		t.Fatal("expected HTTP server header to be hidden by default")
	}
}

func TestDefaultHTTPServerVersionHidden(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	if cfg.HTTP.ExposeServerVersion {
		t.Fatal("expected HTTP server version to be hidden by default")
	}
}
