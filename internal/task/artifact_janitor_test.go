package task_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/lyonbrown4d/spack/internal/artifact"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/task"
	"github.com/samber/oops"
)

func TestSyncArtifactCatalogRemovesOrphanArtifacts(t *testing.T) {
	root := t.TempDir()
	store := newArtifactStoreForTest(root)
	cat := catalog.NewInMemoryCatalog()

	orphanPath := filepath.Join(root, "encoding", "hash-orphan", "app.js.br")
	writeFileForTest(t, orphanPath, []byte("orphan"))

	report, err := task.SyncArtifactCatalogForTest(context.Background(), store, cat, nil)
	if err != nil {
		t.Fatal(err)
	}

	assertOne(t, report.ScannedArtifacts, "scanned artifacts")
	assertOne(t, report.RemovedOrphans, "removed orphan artifacts")
	assertFileRemoved(t, orphanPath)
}

func TestSyncArtifactCatalogRemovesMissingCatalogVariantsAndCache(t *testing.T) {
	root := t.TempDir()
	store := newArtifactStoreForTest(root)
	cat := catalog.NewInMemoryCatalog()
	cache := assetcache.NewCacheForTest(config.MemoryCache{
		Enable:      true,
		MaxEntries:  16,
		MaxFileSize: 64 * 1024,
		TTL:         "5m",
	}, slog.New(slog.DiscardHandler))

	assetPath := filepath.Join(root, "app.js")
	artifactPath := filepath.Join(root, "encoding", "hash-old", "app.js.br")
	writeFileForTest(t, assetPath, []byte("console.log('ok');"))
	writeFileForTest(t, artifactPath, []byte("payload"))

	upsertAssetForTest(t, cat, &catalog.Asset{
		Path:       "app.js",
		FullPath:   assetPath,
		Size:       int64(len("console.log('ok');")),
		MediaType:  "application/javascript",
		SourceHash: "hash-old",
		ETag:       "\"hash-old\"",
	})
	upsertVariantForTest(t, cat, artifactPath)

	if _, _, err := cache.GetOrLoad(artifactPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(artifactPath); err != nil {
		t.Fatal(err)
	}

	report, err := task.SyncArtifactCatalogForTest(context.Background(), store, cat, cache)
	if err != nil {
		t.Fatal(err)
	}

	assertOne(t, report.MissingVariants, "missing variants")
	assertOne(t, report.CacheInvalidations, "cache invalidations")
	if cat.ListVariants("app.js").Len() != 0 {
		t.Fatalf("expected app.js variants to be removed, got %#v", cat.ListVariants("app.js"))
	}

	if _, found, err := cache.GetOrLoad(artifactPath); err == nil || found {
		t.Fatalf("expected cache miss after janitor invalidation, got found=%v err=%v", found, err)
	}
}

func newArtifactStoreForTest(root string) artifact.Store {
	return &testArtifactStore{root: root}
}

type testArtifactStore struct {
	root string
}

func (s *testArtifactStore) Root() string {
	return s.root
}

func (s *testArtifactStore) PathFor(assetPath, sourceHash, namespace, suffix string) (string, error) {
	return filepath.Join(s.root, namespace, sourceHash, assetPath+suffix), nil
}

func (s *testArtifactStore) Write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return oops.In("task").Owner("artifact janitor test").Wrap(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return oops.In("task").Owner("artifact janitor test").Wrap(err)
	}
	return nil
}
