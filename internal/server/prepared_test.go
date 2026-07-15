package server_test

import (
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
	if err := svc.Rebuild(t.Context()); err != nil {
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

func TestPreparedServiceResolvesSimpleEncodingByServerPriority(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	root := t.TempDir()
	assetPath := filepath.Join(root, "app.js")
	brPath := filepath.Join(root, "app.js.br")
	gzipPath := filepath.Join(root, "app.js.gz")
	writePreparedTestFile(t, assetPath, []byte("console.log('app');"))
	writePreparedTestFile(t, brPath, []byte("br"))
	writePreparedTestFile(t, gzipPath, []byte("gzip"))

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
		ArtifactPath: brPath,
		Size:         2,
		MediaType:    "application/javascript",
		SourceHash:   "hash-app",
		ETag:         "\"hash-app-br\"",
		Encoding:     "br",
	})
	upsertPreparedVariant(t, cat, &catalog.Variant{
		ID:           "app.js|encoding=gzip",
		AssetPath:    "app.js",
		ArtifactPath: gzipPath,
		Size:         4,
		MediaType:    "application/javascript",
		SourceHash:   "hash-app",
		ETag:         "\"hash-app-gzip\"",
		Encoding:     "gzip",
	})

	svc := server.NewPreparedServiceForTest(&cfg, slog.New(slog.DiscardHandler), cat)
	if err := svc.Rebuild(t.Context()); err != nil {
		t.Fatal(err)
	}

	selection, ok := server.ResolvePreparedForTest(svc, resolver.Request{
		Path:           "app.js",
		AcceptEncoding: "gzip, br",
	}, "")
	if !ok {
		t.Fatal("expected prepared route")
	}
	if selection.Encoding != "br" {
		t.Fatalf("expected br by server priority, got %q", selection.Encoding)
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
	if err := svc.Rebuild(t.Context()); err != nil {
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

func TestPreparedServiceResolvesDirectoryEntryAlias(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	root := t.TempDir()
	entryPath := filepath.Join(root, "docs", "index.html")
	if err := os.MkdirAll(filepath.Dir(entryPath), 0o750); err != nil {
		t.Fatal(err)
	}
	writePreparedTestFile(t, entryPath, []byte("<html>docs</html>"))

	cat := catalog.NewInMemoryCatalog()
	upsertPreparedAsset(t, cat, &catalog.Asset{
		Path:       "docs/index.html",
		FullPath:   entryPath,
		Size:       int64(len("<html>docs</html>")),
		MediaType:  "text/html; charset=utf-8",
		SourceHash: "hash-docs",
		ETag:       "\"hash-docs\"",
	})

	svc := server.NewPreparedServiceForTest(&cfg, slog.New(slog.DiscardHandler), cat)
	if err := svc.Rebuild(t.Context()); err != nil {
		t.Fatal(err)
	}

	selection, ok := server.ResolvePreparedForTest(svc, resolver.Request{Path: "docs"}, "")
	if !ok {
		t.Fatal("expected prepared directory alias route")
	}
	if selection.FallbackUsed {
		t.Fatal("expected primary alias route, not SPA fallback")
	}
	if selection.FilePath != entryPath {
		t.Fatalf("expected docs entry alias %q, got %q", entryPath, selection.FilePath)
	}
}

func TestPreparedServiceWidthRequestFallsBackToZeroWidthImageVariant(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	root := t.TempDir()
	assetPath := filepath.Join(root, "hero.jpg")
	variantPath := filepath.Join(root, "hero.webp")
	writePreparedTestFile(t, assetPath, []byte("jpeg"))
	writePreparedTestFile(t, variantPath, []byte("webp"))

	cat := catalog.NewInMemoryCatalog()
	upsertPreparedAsset(t, cat, &catalog.Asset{
		Path:       "hero.jpg",
		FullPath:   assetPath,
		Size:       4,
		MediaType:  "image/jpeg",
		SourceHash: "hash-hero",
		ETag:       "\"hash-hero\"",
	})
	upsertPreparedVariant(t, cat, &catalog.Variant{
		ID:           "hero.jpg|format=webp",
		AssetPath:    "hero.jpg",
		ArtifactPath: variantPath,
		Size:         4,
		MediaType:    "image/webp",
		SourceHash:   "hash-hero",
		ETag:         "\"hash-hero-webp\"",
		Format:       "webp",
	})

	svc := server.NewPreparedServiceForTest(&cfg, slog.New(slog.DiscardHandler), cat)
	if err := svc.Rebuild(t.Context()); err != nil {
		t.Fatal(err)
	}

	selection, ok := server.ResolvePreparedForTest(svc, resolver.Request{
		Path:  "hero.jpg",
		Width: 640,
	}, "webp")
	if !ok {
		t.Fatal("expected prepared route")
	}
	if selection.FilePath != variantPath {
		t.Fatalf("expected zero-width webp fallback %q, got %q", variantPath, selection.FilePath)
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
