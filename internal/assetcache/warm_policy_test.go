package assetcache_test

import (
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
)

func TestWarmSkipsLowValueBinaryAssets(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "logo.png")
	writeAssetFile(t, path, []byte("png"))

	cat := catalog.NewInMemoryCatalog()
	upsertAsset(t, cat, &catalog.Asset{
		Path:      "logo.png",
		FullPath:  path,
		Size:      3,
		MediaType: "image/png",
	})

	cache := assetcache.NewCacheForTest(config.MemoryCache{
		Enable:      true,
		Warmup:      true,
		MaxEntries:  16,
		MaxFileSize: 64 * 1024,
		TTL:         "5m",
	}, slog.New(slog.DiscardHandler))

	stats, err := cache.Warm(t.Context(), cat)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Entries != 0 {
		t.Fatalf("expected no warmed entries for low-value binary asset, got %d", stats.Entries)
	}

	assertCacheHitState(t, cache, path, false)
}
