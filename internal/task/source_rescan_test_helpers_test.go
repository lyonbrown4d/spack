package task_test

import (
	"os"
	"path/filepath"
	"testing"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/daiyuang/spack/internal/catalog"
	"github.com/daiyuang/spack/internal/config"
	"github.com/daiyuang/spack/internal/source"
	"log/slog"
)

func newLocalSourceForTest(t *testing.T, root string) source.Source {
	t.Helper()

	src, err := source.NewLocalFSForTest(&config.Assets{Root: root}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func upsertAssetForTest(t *testing.T, cat catalog.Catalog, asset *catalog.Asset) {
	t.Helper()
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}
}

func upsertVariantForTest(t *testing.T, cat catalog.Catalog, artifactPath string) {
	t.Helper()

	writeFileForTest(t, artifactPath, []byte("payload"))
	if err := cat.UpsertVariant(&catalog.Variant{
		ID:           "app.js|encoding=br",
		AssetPath:    "app.js",
		ArtifactPath: artifactPath,
		MediaType:    "application/javascript",
		SourceHash:   "hash-old",
		ETag:         "\"hash-old-br\"",
		Encoding:     "br",
	}); err != nil {
		t.Fatal(err)
	}
}

func writeFileForTest(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertOne(t *testing.T, got int, name string) {
	t.Helper()
	if got != 1 {
		t.Fatalf("expected %s 1, got %d", name, got)
	}
}

func assertFileRemoved(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be deleted, got err=%v", path, err)
	}
}

func singleVariantForTest(t *testing.T, variants *cxlist.List[*catalog.Variant]) *catalog.Variant {
	t.Helper()
	variant, ok := variants.Get(0)
	if !ok || variant == nil {
		t.Fatalf("expected first variant, got %#v", variants)
	}
	return variant
}
