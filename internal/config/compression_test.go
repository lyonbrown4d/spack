package config_test

import (
	"slices"
	"testing"
	"time"

	"github.com/daiyuang/spack/internal/config"
)

func TestCompressionQueueCapacity(t *testing.T) {
	cfg := config.Compression{Workers: 2}
	if got := cfg.QueueCapacity(); got != 128 {
		t.Fatalf("expected queue capacity 128, got %d", got)
	}

	cfg.QueueSize = 32
	if got := cfg.QueueCapacity(); got != 32 {
		t.Fatalf("expected explicit queue capacity 32, got %d", got)
	}
}

func TestCompressionParsedCleanupInterval(t *testing.T) {
	cfg := config.Compression{CleanupEvery: "10m"}
	if got := cfg.ParsedCleanupInterval(); got != 10*time.Minute {
		t.Fatalf("expected 10m, got %s", got)
	}

	cfg.CleanupEvery = "bad"
	if got := cfg.ParsedCleanupInterval(); got != 5*time.Minute {
		t.Fatalf("expected fallback 5m, got %s", got)
	}
}

func TestCompressionParsedMaxAge(t *testing.T) {
	cfg := config.Compression{MaxAge: "72h"}
	if got := cfg.ParsedMaxAge(); got != 72*time.Hour {
		t.Fatalf("expected 72h, got %s", got)
	}

	cfg.MaxAge = "3600"
	if got := cfg.ParsedMaxAge(); got != time.Hour {
		t.Fatalf("expected 1h, got %s", got)
	}

	cfg.MaxAge = "bad"
	if got := cfg.ParsedMaxAge(); got != 0 {
		t.Fatalf("expected 0 for invalid max age, got %s", got)
	}
}

func TestCompressionNamespaceMaxAges(t *testing.T) {
	cfg := config.Compression{
		EncodingMaxAge: "24h",
		ImageMaxAge:    "72h",
	}
	got := cfg.NamespaceMaxAges()
	encodingMaxAge, ok := got.Get("encoding")
	if !ok || encodingMaxAge != 24*time.Hour {
		t.Fatalf("expected encoding max age 24h, got %s", encodingMaxAge)
	}
	imageMaxAge, ok := got.Get("image")
	if !ok || imageMaxAge != 72*time.Hour {
		t.Fatalf("expected image max age 72h, got %s", imageMaxAge)
	}

	cfg = config.Compression{
		EncodingMaxAge: "bad",
		ImageMaxAge:    "0",
	}
	got = cfg.NamespaceMaxAges()
	if got.Len() != 0 {
		t.Fatalf("expected empty namespace max ages, got %#v", got)
	}
}

func TestCompressionNamespaceMaxCacheBytes(t *testing.T) {
	cfg := config.Compression{
		EncodingMaxCacheBytes: 1024,
		ImageMaxCacheBytes:    2048,
	}
	got := cfg.NamespaceMaxCacheBytes()
	encodingMaxCacheBytes, ok := got.Get("encoding")
	if !ok || encodingMaxCacheBytes != 1024 {
		t.Fatalf("expected encoding max cache bytes 1024, got %d", encodingMaxCacheBytes)
	}
	imageMaxCacheBytes, ok := got.Get("image")
	if !ok || imageMaxCacheBytes != 2048 {
		t.Fatalf("expected image max cache bytes 2048, got %d", imageMaxCacheBytes)
	}
}

func TestCompressionNormalizedEncodings(t *testing.T) {
	cfg := config.Compression{Encodings: " gzip , zstd , bad , gzip "}
	if got := cfg.NormalizedEncodings(); !slices.Equal(got.Values(), []string{"gzip", "zstd"}) {
		t.Fatalf("expected normalized encodings [gzip zstd], got %#v", got.Values())
	}

	cfg = config.Compression{Encodings: "bad"}
	if got := cfg.NormalizedEncodings(); !slices.Equal(got.Values(), []string{"br", "zstd", "gzip"}) {
		t.Fatalf("expected default encodings [br zstd gzip], got %#v", got.Values())
	}
}
