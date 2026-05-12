package assetcache_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/arcgolabs/eventx"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	appEvent "github.com/lyonbrown4d/spack/internal/event"
)

func TestShouldServe(t *testing.T) {
	cache := assetcache.NewCacheForTest(config.MemoryCache{
		Enable:      true,
		MaxEntries:  16,
		MaxFileSize: 64 * 1024,
		TTL:         "5m",
	}, slog.New(slog.DiscardHandler))

	if !cache.ShouldServe(1024, false) {
		t.Fatal("expected small asset to be served from memory")
	}
	if cache.ShouldServe(1024, true) {
		t.Fatal("expected range request to bypass memory cache")
	}
	if cache.ShouldServe(128*1024, false) {
		t.Fatal("expected oversized asset to bypass memory cache")
	}
}

func TestGetOrLoad(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "asset.js")
	payload := []byte("console.log('cached');")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	obs := &recordingObservability{}
	cache := assetcache.NewCacheWithObservabilityForTest(config.MemoryCache{
		Enable:      true,
		MaxEntries:  16,
		MaxFileSize: 64 * 1024,
		TTL:         "5m",
	}, slog.New(slog.DiscardHandler), obs)

	body, found, err := cache.GetOrLoad(path)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected first load to miss memory cache")
	}
	if !bytes.Equal(body, payload) {
		t.Fatalf("expected payload %q, got %q", payload, body)
	}

	body, found, err = cache.GetOrLoad(path)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected second load to hit memory cache")
	}
	if !bytes.Equal(body, payload) {
		t.Fatalf("expected cached payload %q, got %q", payload, body)
	}

	assertCounterValue(t, obs, "asset_cache_misses_total", 1)
	assertCounterValue(t, obs, "asset_cache_hits_total", 1)
	assertCounterValue(t, obs, "asset_cache_fills_total", 1)
	assertCounterValue(t, obs, "asset_cache_fill_bytes_total", int64(len(payload)))
}

func TestWarm(t *testing.T) {
	root := t.TempDir()
	smallPath := filepath.Join(root, "index.html")
	largePath := filepath.Join(root, "big.js")
	writeAssetFile(t, smallPath, []byte("<html>ok</html>"))
	writeAssetFile(t, largePath, bytes.Repeat([]byte("a"), 128*1024))

	cat := catalog.NewInMemoryCatalog()
	upsertAsset(t, cat, &catalog.Asset{
		Path:     "index.html",
		FullPath: smallPath,
		Size:     15,
	})
	upsertAsset(t, cat, &catalog.Asset{
		Path:     "big.js",
		FullPath: largePath,
		Size:     128 * 1024,
	})

	obs := &recordingObservability{}
	cache := assetcache.NewCacheWithObservabilityForTest(config.MemoryCache{
		Enable:      true,
		Warmup:      true,
		MaxEntries:  16,
		MaxFileSize: 64 * 1024,
		TTL:         "5m",
	}, slog.New(slog.DiscardHandler), obs)

	stats, err := cache.Warm(context.Background(), cat)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Entries != 1 {
		t.Fatalf("expected one warmed entry, got %d", stats.Entries)
	}

	assertCacheHitState(t, cache, smallPath, true)
	assertCacheHitState(t, cache, largePath, false)
	assertCounterValue(t, obs, "asset_cache_warm_entries_total", 1)
	assertCounterValue(t, obs, "asset_cache_warm_bytes_total", 15)
}

func TestVariantRemovedEventInvalidatesCacheEntry(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "asset.js")
	payload := []byte("console.log('invalidate');")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	bus := eventx.New()
	cache := assetcache.NewCacheWithBusForTest(config.MemoryCache{
		Enable:      true,
		MaxEntries:  16,
		MaxFileSize: 64 * 1024,
		TTL:         "5m",
	}, slog.New(slog.DiscardHandler), nil, bus)
	if err := assetcache.StartForTest(cache); err != nil {
		t.Fatal(err)
	}

	if _, found, err := cache.GetOrLoad(path); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatal("expected first load to miss memory cache")
	}
	if _, found, err := cache.GetOrLoad(path); err != nil {
		t.Fatal(err)
	} else if !found {
		t.Fatal("expected second load to hit memory cache")
	}

	if err := bus.Publish(context.Background(), appEvent.VariantRemoved{
		ArtifactPath: path,
		Reason:       appEvent.VariantRemovalReasonTTL,
	}); err != nil {
		t.Fatal(err)
	}

	if _, found, err := cache.GetOrLoad(path); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatal("expected load after invalidation to miss memory cache")
	}
}

func TestVariantGeneratedEventPreloadsCacheEntry(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "asset.js.br")
	payload := []byte("br")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	bus := eventx.New()
	cache := assetcache.NewCacheWithBusForTest(config.MemoryCache{
		Enable:      true,
		MaxEntries:  16,
		MaxFileSize: 64 * 1024,
		TTL:         "5m",
	}, slog.New(slog.DiscardHandler), nil, bus)
	if err := assetcache.StartForTest(cache); err != nil {
		t.Fatal(err)
	}

	if err := bus.Publish(context.Background(), appEvent.VariantGenerated{
		AssetPath:    "asset.js",
		ArtifactPath: path,
		Stage:        "compression",
		Size:         int64(len(payload)),
	}); err != nil {
		t.Fatal(err)
	}

	if _, found, err := cache.GetOrLoad(path); err != nil {
		t.Fatal(err)
	} else if !found {
		t.Fatal("expected first load after generated event to hit memory cache")
	}
}

func writeAssetFile(t *testing.T, path string, body []byte) {
	t.Helper()

	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func upsertAsset(t *testing.T, cat catalog.Catalog, asset *catalog.Asset) {
	t.Helper()

	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}
}

func assertCacheHitState(t *testing.T, cache *assetcache.Cache, path string, wantHit bool) {
	t.Helper()

	_, found, err := cache.GetOrLoad(path)
	if err != nil {
		t.Fatal(err)
	}
	if found != wantHit {
		t.Fatalf("expected cache hit=%v for %s, got %v", wantHit, path, found)
	}
}
