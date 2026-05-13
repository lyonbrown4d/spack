package catalog_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lyonbrown4d/spack/internal/catalog"
)

func TestUpsertVariantBackfillsModTimeMetadata(t *testing.T) {
	cat := catalog.NewInMemoryCatalog()
	root := t.TempDir()
	assetPath := filepath.Join(root, "app.js")
	variantPath := filepath.Join(root, "app.js.br")
	modifiedAt := time.Unix(1_720_000_654, 0).UTC()

	writeCatalogMetadataFile(t, assetPath, []byte("console.log('app');"))
	writeCatalogMetadataFile(t, variantPath, []byte("br"))
	if err := os.Chtimes(variantPath, modifiedAt, modifiedAt); err != nil {
		t.Fatal(err)
	}

	if err := cat.UpsertAsset(&catalog.Asset{
		Path:       "app.js",
		FullPath:   assetPath,
		MediaType:  "application/javascript",
		SourceHash: "hash-1",
		ETag:       "\"hash-1\"",
	}); err != nil {
		t.Fatal(err)
	}
	if err := cat.UpsertVariant(&catalog.Variant{
		ID:           "app.js|encoding=br",
		AssetPath:    "app.js",
		ArtifactPath: variantPath,
		MediaType:    "application/javascript",
		SourceHash:   "hash-1",
		ETag:         "\"hash-1-br\"",
		Encoding:     "br",
	}); err != nil {
		t.Fatal(err)
	}

	variant, ok := cat.FindEncodingVariant("app.js", "br")
	if !ok {
		t.Fatal("expected variant to be present")
	}
	if got := variant.Metadata.GetOrDefault(catalog.MetadataModTimeUnixKey, ""); got != "1720000654" {
		t.Fatalf("expected backfilled mod time metadata, got %q", got)
	}
	if got := variant.Metadata.GetOrDefault(catalog.MetadataLastModifiedHTTPKey, ""); got != modifiedAt.Format(http.TimeFormat) {
		t.Fatalf("expected backfilled http last-modified metadata, got %q", got)
	}
}

func writeCatalogMetadataFile(t *testing.T, path string, body []byte) {
	t.Helper()

	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}
