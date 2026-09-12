package assetcache_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/config"
)

func BenchmarkCacheGetOrLoadMiss(b *testing.B) {
	benchmarkCacheGetOrLoad(b, false, false)
}

func BenchmarkCacheGetOrLoadObservedMiss(b *testing.B) {
	benchmarkCacheGetOrLoad(b, true, false)
}

func BenchmarkCacheGetOrLoadHit(b *testing.B) {
	benchmarkCacheGetOrLoad(b, false, true)
}

func BenchmarkCacheGetOrLoadObservedHit(b *testing.B) {
	benchmarkCacheGetOrLoad(b, true, true)
}

type benchmarkCache interface {
	Delete(path string) bool
	GetOrLoad(path string) ([]byte, bool, error)
}

func benchmarkCacheGetOrLoad(b *testing.B, observed, hit bool) {
	b.Helper()

	payload := bytes.Repeat([]byte("a"), 16*1024)
	root := b.TempDir()
	path := filepath.Join(root, "asset.js")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		b.Fatal(err)
	}

	cacheConfig := config.MemoryCache{
		Enable:      true,
		MaxEntries:  16,
		MaxFileSize: 64 * 1024,
		TTL:         "5m",
	}
	logger := slog.New(slog.DiscardHandler)
	cache := assetcache.NewCacheForTest(cacheConfig, logger)
	if observed {
		cache = assetcache.NewCacheWithObservabilityForTest(
			cacheConfig,
			logger,
			observabilityx.Nop(),
		)
	}

	if err := prepareBenchmarkCache(cache, path, hit); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()

	for b.Loop() {
		if err := benchmarkCacheGetOrLoadOnce(cache, path, len(payload), hit); err != nil {
			b.Fatal(err)
		}
	}
}

func prepareBenchmarkCache(cache benchmarkCache, path string, hit bool) error {
	if !hit {
		return nil
	}

	_, found, err := cache.GetOrLoad(path)
	if err != nil {
		return fmt.Errorf("prime benchmark cache: %w", err)
	}
	if found {
		return errors.New("expected first load to miss memory cache")
	}

	return nil
}

func benchmarkCacheGetOrLoadOnce(cache benchmarkCache, path string, payloadLength int, hit bool) error {
	if !hit {
		_ = cache.Delete(path)
	}

	body, found, err := cache.GetOrLoad(path)
	if err != nil {
		return fmt.Errorf("get benchmark cache entry: %w", err)
	}
	if found != hit {
		return fmt.Errorf("expected cache hit=%t, got %t", hit, found)
	}
	if len(body) != payloadLength {
		return fmt.Errorf("expected payload length %d, got %d", payloadLength, len(body))
	}

	return nil
}
