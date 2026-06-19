package sourcecatalog_test

import (
	"context"
	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScannerRecognizesEnabledCompressionSidecars(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, filepath.Join(root, "app.js"), []byte("console.log('ok');"))
	sidecarPath := filepath.Join(root, "app.js.br")
	writeSourceFile(t, sidecarPath, []byte("compressed"))

	scanner := newScannerForTest(t, root, config.DefaultConfigForTest().Compression.NormalizedEncodings())
	snapshot, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := snapshot.Assets.Get("app.js"); !ok {
		t.Fatal("expected app.js asset to be scanned")
	}
	if _, ok := snapshot.Assets.Get("app.js.br"); ok {
		t.Fatal("expected app.js.br to be recognized as sidecar variant, not asset")
	}

	variant, ok := snapshot.Variants.Get("app.js.br")
	if !ok {
		t.Fatal("expected app.js.br variant to be registered")
	}
	if variant.ArtifactPath != sidecarPath {
		t.Fatalf("expected sidecar artifact path %q, got %q", sidecarPath, variant.ArtifactPath)
	}
	if variant.Encoding != "br" {
		t.Fatalf("expected br encoding, got %q", variant.Encoding)
	}
	if !sourcecatalog.IsSourceSidecarVariant(variant) {
		t.Fatal("expected sidecar variant metadata to mark source sidecar stage")
	}
}

func TestScannerLeavesDisabledEncodingSidecarsAsAssets(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, filepath.Join(root, "app.js"), []byte("console.log('ok');"))
	writeSourceFile(t, filepath.Join(root, "app.js.br"), []byte("compressed"))

	scanner := newScannerForTest(t, root, cxlist.NewList("gzip"))
	snapshot, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := snapshot.Assets.Get("app.js.br"); !ok {
		t.Fatal("expected disabled sidecar suffix to remain a plain asset")
	}
	if snapshot.Variants.Len() != 0 {
		t.Fatalf("expected no recognized variants, got %d", snapshot.Variants.Len())
	}
}

func TestScannerReusesUnchangedAssetFromCatalog(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "app.js")
	assetBody := []byte("console.log('ok');")
	writeSourceFile(t, assetPath, assetBody)
	modTime := setFileModTimeForTest(t, assetPath, time.Unix(1_720_000_321, 123_000_000).UTC())

	scanner := newScannerForTest(t, root, config.DefaultConfigForTest().Compression.NormalizedEncodings())
	cat := catalog.NewInMemoryCatalog()
	if err := cat.UpsertAsset(&catalog.Asset{
		Path:       "app.js",
		FullPath:   assetPath,
		Size:       int64(len(assetBody)),
		MediaType:  "application/javascript",
		SourceHash: "hash-app",
		ETag:       "\"hash-app\"",
		Metadata:   catalog.MetadataWithModTime(cxmapping.NewMap[string, string](), modTime),
	}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := scanner.ScanWithCatalog(context.Background(), cat)
	if err != nil {
		t.Fatal(err)
	}

	asset, ok := snapshot.Assets.Get("app.js")
	if !ok || asset == nil {
		t.Fatal("expected app.js asset to be reused from catalog")
	}
	if asset.SourceHash != "hash-app" {
		t.Fatalf("expected reused source hash, got %q", asset.SourceHash)
	}
}

func TestScannerReusesUnchangedSourceSidecarFromCatalog(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "app.js")
	sidecarPath := filepath.Join(root, "app.js.br")
	assetBody := []byte("console.log('ok');")
	sidecarBody := []byte("compressed")
	writeSourceFile(t, assetPath, assetBody)
	writeSourceFile(t, sidecarPath, sidecarBody)
	modTime := time.Unix(1_720_000_322, 456_000_000).UTC()
	assetModTime := setFileModTimeForTest(t, assetPath, modTime)
	sidecarModTime := setFileModTimeForTest(t, sidecarPath, modTime)

	scanner := newScannerForTest(t, root, config.DefaultConfigForTest().Compression.NormalizedEncodings())
	cat := catalog.NewInMemoryCatalog()
	if err := cat.UpsertAsset(&catalog.Asset{
		Path:       "app.js",
		FullPath:   assetPath,
		Size:       int64(len(assetBody)),
		MediaType:  "application/javascript",
		SourceHash: "hash-app",
		ETag:       "\"hash-app\"",
		Metadata:   catalog.MetadataWithModTime(cxmapping.NewMap[string, string](), assetModTime),
	}); err != nil {
		t.Fatal(err)
	}
	if err := cat.UpsertVariant(&catalog.Variant{
		ID:           "app.js.br",
		AssetPath:    "app.js",
		ArtifactPath: sidecarPath,
		Size:         int64(len(sidecarBody)),
		MediaType:    "application/javascript",
		SourceHash:   "hash-app",
		ETag:         "\"hash-sidecar\"",
		Encoding:     "br",
		Metadata: catalog.MetadataWithModTime(cxmapping.NewMapFrom(map[string]string{
			"stage":  sourcecatalog.SourceSidecarStage,
			"source": "app.js.br",
		}), sidecarModTime),
	}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := scanner.ScanWithCatalog(context.Background(), cat)
	if err != nil {
		t.Fatal(err)
	}

	variant, ok := snapshot.Variants.Get("app.js.br")
	if !ok || variant == nil {
		t.Fatal("expected app.js.br sidecar variant to be reused from catalog")
	}
	if variant.ETag != "\"hash-sidecar\"" {
		t.Fatalf("expected reused sidecar etag, got %q", variant.ETag)
	}
}

func newScannerForTest(t *testing.T, root string, encodings *cxlist.List[string]) sourcecatalog.Scanner {
	t.Helper()

	src, err := source.NewLocalFS(&config.Assets{Root: root}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}

	return newScannerFromSource(src, encodings)
}

func newScannerFromSource(src *source.LocalFS, encodings *cxlist.List[string]) sourcecatalog.Scanner {
	cfg := config.DefaultConfigForTest()
	return sourcecatalog.NewScanner(src, contentcoding.NewRegistry(contentcoding.Options{
		BrotliQuality: cfg.Compression.BrotliQuality,
		GzipLevel:     cfg.Compression.GzipLevel,
		ZstdLevel:     cfg.Compression.ZstdLevel,
	}, encodings))
}

func writeSourceFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func setFileModTimeForTest(t *testing.T, path string, modTime time.Time) time.Time {
	t.Helper()
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.ModTime()
}
