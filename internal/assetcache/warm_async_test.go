package assetcache_test

import (
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
)

func TestWarmWithSharedWorkerSettings(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "index.html")
	secondPath := filepath.Join(root, "about.html")
	writeAssetFile(t, firstPath, []byte("<html>home</html>"))
	writeAssetFile(t, secondPath, []byte("<html>about</html>"))

	cat := catalog.NewInMemoryCatalog()
	upsertAsset(t, cat, &catalog.Asset{
		Path:     "index.html",
		FullPath: firstPath,
		Size:     int64(len("<html>home</html>")),
	})
	upsertAsset(t, cat, &catalog.Asset{
		Path:     "about.html",
		FullPath: secondPath,
		Size:     int64(len("<html>about</html>")),
	})

	obs := &recordingObservability{}
	cache := assetcache.NewCacheWithSettingsForTest(config.MemoryCache{
		Enable:      true,
		Warmup:      true,
		MaxEntries:  16,
		MaxFileSize: 64 * 1024,
		TTL:         "5m",
	}, slog.New(slog.DiscardHandler), obs, &asyncx.Settings{Size: 2})

	stats, err := cache.Warm(t.Context(), cat)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Entries != 2 {
		t.Fatalf("expected two warmed entries, got %d", stats.Entries)
	}

	assertCacheHitState(t, cache, firstPath, true)
	assertCacheHitState(t, cache, secondPath, true)
	assertCounterValue(t, obs, "asset_cache_warm_entries_total", 2)
	assertCounterValue(t, obs, "asset_cache_warm_bytes_total", int64(len("<html>home</html>")+len("<html>about</html>")))
}
