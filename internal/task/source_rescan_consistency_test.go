package task_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/task"
)

func TestSyncSourceCatalogInvalidatesCatalogMemoryCacheAndArtifactsOnRename(t *testing.T) {
	root := t.TempDir()
	artifactRoot := t.TempDir()
	oldAssetPath := filepath.Join(root, "app.js")
	newAssetPath := filepath.Join(root, "renamed.js")
	artifactPath := filepath.Join(artifactRoot, "app.js.br")
	writeFileForTest(t, oldAssetPath, []byte("console.log('old');"))

	src := newLocalSourceForTest(t, root)
	cat := catalog.NewInMemoryCatalog()
	bodyCache := assetcache.NewCacheForTest(config.MemoryCache{
		Enable:      true,
		MaxEntries:  16,
		MaxFileSize: 64 * 1024,
		TTL:         "5m",
	}, slog.New(slog.DiscardHandler))

	upsertAssetForTest(t, cat, &catalog.Asset{
		Path:       "app.js",
		FullPath:   oldAssetPath,
		Size:       int64(len("console.log('old');")),
		MediaType:  "application/javascript",
		SourceHash: "hash-old",
		ETag:       "\"hash-old\"",
	})
	upsertVariantForTest(t, cat, artifactPath)
	warmCacheEntryForTest(t, bodyCache, oldAssetPath)
	warmCacheEntryForTest(t, bodyCache, artifactPath)

	if err := os.Rename(oldAssetPath, newAssetPath); err != nil {
		t.Fatal(err)
	}

	report, err := task.SyncSourceCatalogForTest(context.Background(), src, cat, bodyCache, source.ChangeEvent{
		Path: "app.js",
		Op:   "RENAME",
	})
	if err != nil {
		t.Fatal(err)
	}

	assertOne(t, report.Added, "added renamed asset")
	assertOne(t, report.Removed, "removed old asset")
	assertOne(t, report.RemovedVariants, "removed stale variant")
	assertOne(t, report.RemovedArtifacts, "removed stale artifact")
	if report.CacheInvalidations != 2 {
		t.Fatalf("expected two memory cache invalidations, got %d", report.CacheInvalidations)
	}
	if _, ok := cat.FindAsset("app.js"); ok {
		t.Fatal("expected old app.js asset to be removed from catalog")
	}
	if _, ok := cat.FindAsset("renamed.js"); !ok {
		t.Fatal("expected renamed.js asset to be added to catalog")
	}
	if cat.ListVariants("app.js").Len() != 0 {
		t.Fatalf("expected stale app.js variants to be removed, got %#v", cat.ListVariants("app.js"))
	}
	assertFileRemoved(t, artifactPath)
	assertCacheEntryRemovedForTest(t, bodyCache, oldAssetPath)
	assertCacheEntryRemovedForTest(t, bodyCache, artifactPath)
}

func warmCacheEntryForTest(t *testing.T, bodyCache *assetcache.Cache, path string) {
	t.Helper()

	if _, found, err := bodyCache.GetOrLoad(path); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatalf("expected initial cache load for %s to miss", path)
	}
	if _, found := bodyCache.GetCachedEntry(path); !found {
		t.Fatalf("expected %s to be cached", path)
	}
}

func assertCacheEntryRemovedForTest(t *testing.T, bodyCache *assetcache.Cache, path string) {
	t.Helper()

	if _, found := bodyCache.GetCachedEntry(path); found {
		t.Fatalf("expected memory cache entry %s to be invalidated", path)
	}
}
