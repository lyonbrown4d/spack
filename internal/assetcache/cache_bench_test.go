package assetcache_test

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/config"
)

func BenchmarkCacheGetOrLoadMiss(b *testing.B) {
	payload := bytes.Repeat([]byte("a"), 16*1024)
	root := b.TempDir()
	path := filepath.Join(root, "asset.js")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		b.Fatal(err)
	}

	cache := assetcache.NewCacheForTest(config.MemoryCache{
		Enable:      true,
		MaxEntries:  16,
		MaxFileSize: 64 * 1024,
		TTL:         "5m",
	}, slog.New(slog.DiscardHandler))

	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()

	for b.Loop() {
		cache.Delete(path)
		body, found, err := cache.GetOrLoad(path)
		if err != nil {
			b.Fatal(err)
		}
		if found {
			b.Fatal("expected miss after deleting cache entry")
		}
		if len(body) != len(payload) {
			b.Fatalf("expected payload length %d, got %d", len(payload), len(body))
		}
	}
}

func BenchmarkCacheGetOrLoadHit(b *testing.B) {
	payload := bytes.Repeat([]byte("a"), 16*1024)
	root := b.TempDir()
	path := filepath.Join(root, "asset.js")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		b.Fatal(err)
	}

	cache := assetcache.NewCacheForTest(config.MemoryCache{
		Enable:      true,
		MaxEntries:  16,
		MaxFileSize: 64 * 1024,
		TTL:         "5m",
	}, slog.New(slog.DiscardHandler))

	if _, found, err := cache.GetOrLoad(path); err != nil {
		b.Fatal(err)
	} else if found {
		b.Fatal("expected first load to miss memory cache")
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()

	for b.Loop() {
		body, found, err := cache.GetOrLoad(path)
		if err != nil {
			b.Fatal(err)
		}
		if !found {
			b.Fatal("expected cached response")
		}
		if len(body) != len(payload) {
			b.Fatalf("expected payload length %d, got %d", len(payload), len(body))
		}
	}
}
