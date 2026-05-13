package server_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/server"
)

func TestPreparedServiceResolvesEncodingVariantWithoutResolver(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	root := t.TempDir()
	assetPath := filepath.Join(root, "app.js")
	variantPath := filepath.Join(root, "app.js.br")
	writePreparedTestFile(t, assetPath, []byte("console.log('app');"))
	writePreparedTestFile(t, variantPath, []byte("br"))

	cat := catalog.NewInMemoryCatalog()
	upsertPreparedAsset(t, cat, &catalog.Asset{
		Path:       "app.js",
		FullPath:   assetPath,
		Size:       int64(len("console.log('app');")),
		MediaType:  "application/javascript",
		SourceHash: "hash-app",
		ETag:       "\"hash-app\"",
	})
	upsertPreparedVariant(t, cat, &catalog.Variant{
		ID:           "app.js|encoding=br",
		AssetPath:    "app.js",
		ArtifactPath: variantPath,
		Size:         2,
		MediaType:    "application/javascript",
		SourceHash:   "hash-app",
		ETag:         "\"hash-app-br\"",
		Encoding:     "br",
	})

	svc := server.NewPreparedServiceForTest(&cfg, slog.New(slog.DiscardHandler), cat)
	if err := svc.Rebuild(context.Background()); err != nil {
		t.Fatal(err)
	}

	selection, ok := server.ResolvePreparedForTest(svc, resolver.Request{
		Path:           "app.js",
		AcceptEncoding: "br,gzip;q=0.5",
	}, "")
	if !ok {
		t.Fatal("expected prepared route")
	}
	if selection.Encoding != "br" {
		t.Fatalf("expected br variant, got %q", selection.Encoding)
	}
	if selection.BodyLen != 2 {
		t.Fatalf("expected prepared variant body, got %d bytes", selection.BodyLen)
	}
}

func TestPreparedServiceFallsBackToEntryRoute(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	root := t.TempDir()
	entryPath := filepath.Join(root, "index.html")
	writePreparedTestFile(t, entryPath, []byte("<html></html>"))

	cat := catalog.NewInMemoryCatalog()
	upsertPreparedAsset(t, cat, &catalog.Asset{
		Path:       "index.html",
		FullPath:   entryPath,
		Size:       int64(len("<html></html>")),
		MediaType:  "text/html; charset=utf-8",
		SourceHash: "hash-index",
		ETag:       "\"hash-index\"",
	})

	svc := server.NewPreparedServiceForTest(&cfg, slog.New(slog.DiscardHandler), cat)
	if err := svc.Rebuild(context.Background()); err != nil {
		t.Fatal(err)
	}

	selection, ok := server.ResolvePreparedForTest(svc, resolver.Request{Path: "about"}, "")
	if !ok {
		t.Fatal("expected fallback route")
	}
	if !selection.FallbackUsed {
		t.Fatal("expected fallback flag")
	}
	if selection.FilePath != entryPath {
		t.Fatalf("expected index fallback, got %q", selection.FilePath)
	}
}

func writePreparedTestFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func upsertPreparedAsset(t *testing.T, cat catalog.Catalog, asset *catalog.Asset) {
	t.Helper()
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}
}

func upsertPreparedVariant(t *testing.T, cat catalog.Catalog, variant *catalog.Variant) {
	t.Helper()
	if err := cat.UpsertVariant(variant); err != nil {
		t.Fatal(err)
	}
}
