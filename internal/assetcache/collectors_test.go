package assetcache_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestCacheCollectorsReportCurrentState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.js")
	body := []byte("console.log('ok');")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}

	cache := assetcache.NewCacheForTest(config.MemoryCache{
		Enable:      true,
		Warmup:      false,
		MaxEntries:  16,
		MaxBytes:    1024,
		MaxFileSize: 1024,
		TTL:         "1m",
	}, slog.New(slog.DiscardHandler))
	if _, found, err := cache.GetOrLoad(path); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatal("expected first load to fill cache")
	}

	registry := prometheus.NewRegistry()
	for _, collector := range cache.Collectors() {
		if err := registry.Register(collector); err != nil {
			t.Fatal(err)
		}
	}

	if got := testutil.ToFloat64(cache.Collectors()[0]); got != 1 {
		t.Fatalf("expected one cache entry, got %v", got)
	}
	if got := testutil.ToFloat64(cache.Collectors()[1]); got != float64(len(body)) {
		t.Fatalf("expected current cache bytes %d, got %v", len(body), got)
	}
	if got := testutil.ToFloat64(cache.Collectors()[2]); got != 1024 {
		t.Fatalf("expected max cache bytes 1024, got %v", got)
	}
}
