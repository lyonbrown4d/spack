package pipeline_test

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/daiyuang/spack/internal/catalog"
	"github.com/daiyuang/spack/internal/config"
	"github.com/daiyuang/spack/internal/pipeline"
)

type namespaceArtifacts struct {
	encodingOld string
	encodingNew string
	imageOld    string
	imageNew    string
}

func setupNamespaceMaxCacheBytesCase(
	t *testing.T,
	root string,
	now time.Time,
) (catalog.Catalog, namespaceArtifacts) {
	t.Helper()
	cat := catalog.NewInMemoryCatalog()
	addAssetForCleanupTest(t, cat, "bundle.js", "application/javascript", "hash-6")
	addAssetForCleanupTest(t, cat, "hero.png", "image/png", "hash-7")

	artifacts := namespaceArtifacts{
		encodingOld: filepath.Join(root, "encoding", "hash-6", "bundle.js.br"),
		encodingNew: filepath.Join(root, "encoding", "hash-6", "bundle.js.gz"),
		imageOld:    filepath.Join(root, "image", "hash-7", "hero.png.jpeg"),
		imageNew:    filepath.Join(root, "image", "hash-7", "hero.png.png"),
	}

	for _, path := range []struct {
		path string
		age  time.Duration
	}{
		{artifacts.encodingOld, 4 * time.Hour},
		{artifacts.encodingNew, 3 * time.Hour},
		{artifacts.imageOld, 2 * time.Hour},
		{artifacts.imageNew, 1 * time.Hour},
	} {
		writeArtifactForCleanupTest(t, path.path, []byte("0123456789abcdef"))
		setMTimeForCleanupTest(t, path.path, now.Add(-path.age))
	}

	variants := []catalog.Variant{
		{
			ID:           "bundle.js|encoding=br",
			AssetPath:    "bundle.js",
			ArtifactPath: artifacts.encodingOld,
			MediaType:    "application/javascript",
			SourceHash:   "hash-6",
			ETag:         "\"hash-6-br\"",
			Encoding:     "br",
		},
		{
			ID:           "bundle.js|encoding=gzip",
			AssetPath:    "bundle.js",
			ArtifactPath: artifacts.encodingNew,
			MediaType:    "application/javascript",
			SourceHash:   "hash-6",
			ETag:         "\"hash-6-gzip\"",
			Encoding:     "gzip",
		},
		{
			ID:           "hero.png|format=jpeg",
			AssetPath:    "hero.png",
			ArtifactPath: artifacts.imageOld,
			MediaType:    "image/jpeg",
			SourceHash:   "hash-7",
			ETag:         "\"hash-7-jpeg\"",
			Format:       "jpeg",
		},
		{
			ID:           "hero.png|format=png",
			AssetPath:    "hero.png",
			ArtifactPath: artifacts.imageNew,
			MediaType:    "image/png",
			SourceHash:   "hash-7",
			ETag:         "\"hash-7-png\"",
			Format:       "png",
		},
	}
	for i := range variants {
		upsertVariantForCleanupTest(t, cat, &variants[i])
	}

	return cat, artifacts
}

func newNamespaceCleanupService(t *testing.T, root string, catalogStore catalog.Catalog) *pipeline.Service {
	t.Helper()
	return pipeline.NewServiceForTest(&config.Compression{
		CacheDir:              root,
		MaxCacheBytes:         0,
		EncodingMaxCacheBytes: 16,
		ImageMaxCacheBytes:    16,
	}, slog.New(slog.DiscardHandler), catalogStore, 1)
}

func addAssetForCleanupTest(t *testing.T, cat catalog.Catalog, path, mediaType, sourceHash string) {
	t.Helper()
	if err := cat.UpsertAsset(&catalog.Asset{
		Path:       path,
		FullPath:   filepath.Join(t.TempDir(), path),
		MediaType:  mediaType,
		SourceHash: sourceHash,
	}); err != nil {
		t.Fatal(err)
	}
}

func upsertVariantForCleanupTest(t *testing.T, cat catalog.Catalog, variant *catalog.Variant) {
	t.Helper()
	if err := cat.UpsertVariant(variant); err != nil {
		t.Fatal(err)
	}
}

func writeArtifactForCleanupTest(t *testing.T, path string, payload []byte) {
	t.Helper()
	mkdirAllForCleanupTest(t, filepath.Dir(path))
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
}

func setMTimeForCleanupTest(t *testing.T, path string, modTime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}

func assertFileStateForCleanupTest(t *testing.T, path string, expectExists bool) {
	t.Helper()
	_, err := os.Stat(path)
	if expectExists {
		if err != nil {
			t.Fatalf("expected path exists: %s, err=%v", path, err)
		}
		return
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected path removed: %s, err=%v", path, err)
	}
}

func mkdirAllForCleanupTest(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(fmt.Errorf("create artifact parent: %w", err))
	}
}
