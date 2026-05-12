package resolver_test

import (
	"context"
	"errors"
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const testSourceHash = "hash-1"

func TestParseAcceptImageFormatsPriority(t *testing.T) {
	got := resolver.ParseAcceptImageFormatsForTest("image/png;q=1,image/jpeg;q=0.6,*/*;q=0.1", "jpeg")
	if !slices.Equal(got.Values(), []string{"png", "jpeg"}) {
		t.Fatalf("unexpected image formats: %#v", got)
	}
}

func TestParseAcceptImageFormatsPrefersExplicitOverWildcard(t *testing.T) {
	got := resolver.ParseAcceptImageFormatsForTest("image/jpeg;q=0.7,image/*;q=0.9", "png")
	if !slices.Equal(got.Values(), []string{"jpeg", "png"}) {
		t.Fatalf("unexpected image formats: %#v", got)
	}
}

func TestResolverFallsBackForSPAPath(t *testing.T) {
	sourcePath, _, assetResolver := newResolverFixture(t, "index.html", "text/html; charset=utf-8", []byte("<html>origin</html>"), spaAssetsConfig())
	result, err := assetResolver.Resolve(context.Background(), resolver.Request{Path: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.FallbackUsed {
		t.Fatal("expected fallback to be used")
	}
	if result.FilePath != sourcePath {
		t.Fatalf("expected fallback path %q, got %q", sourcePath, result.FilePath)
	}
}

func TestResolverDoesNotFallbackForMissingAssetPath(t *testing.T) {
	_, _, assetResolver := newResolverFixture(t, "index.html", "text/html; charset=utf-8", []byte("<html>origin</html>"), spaAssetsConfig())

	_, err := assetResolver.Resolve(context.Background(), resolver.Request{Path: "assets/index-missing.js"})
	if err == nil {
		t.Fatal("expected missing asset path not to use SPA fallback")
	}
	if !errors.Is(err, resolver.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestResolverResolvesRootToEntry(t *testing.T) {
	sourcePath, _, assetResolver := newResolverFixture(t, "index.html", "text/html; charset=utf-8", []byte("<html>origin</html>"), spaAssetsConfig())
	result, err := assetResolver.Resolve(context.Background(), resolver.Request{Path: "/"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Asset == nil || result.Asset.Path != "index.html" {
		t.Fatalf("expected root path to resolve to index.html, got %#v", result.Asset)
	}
	if result.FilePath != sourcePath {
		t.Fatalf("expected entry path %q, got %q", sourcePath, result.FilePath)
	}
	if result.FallbackUsed {
		t.Fatal("expected root path to resolve via entry, not fallback")
	}
}

func TestResolverSelectsWidthVariant(t *testing.T) {
	sourcePath, cat, assetResolver := newResolverFixture(t, "hero.jpg", "image/jpeg", []byte("origin"), baseAssetsConfig())
	variantPath := filepath.Join(filepath.Dir(sourcePath), "hero.w640.jpg")
	writeTestFile(t, variantPath, []byte("scaled"))
	upsertTestVariant(t, cat, &catalog.Variant{
		ID:           "hero.jpg|width=640",
		AssetPath:    "hero.jpg",
		ArtifactPath: variantPath,
		MediaType:    "image/jpeg",
		SourceHash:   testSourceHash,
		ETag:         "\"hash-1-w640\"",
		Width:        640,
	})

	result, err := assetResolver.Resolve(context.Background(), resolver.Request{Path: "hero.jpg", Width: 320})
	if err != nil {
		t.Fatal(err)
	}
	if result.Variant == nil || result.Variant.Width != 640 {
		t.Fatalf("expected 640 width variant, got %#v", result.Variant)
	}
}

func TestResolverRequestsWidthGenerationWhenMissing(t *testing.T) {
	_, _, assetResolver := newResolverFixture(t, "hero.jpg", "image/jpeg", []byte("origin"), baseAssetsConfig())
	result, err := assetResolver.Resolve(context.Background(), resolver.Request{Path: "hero.jpg", Width: 640})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.PreferredWidths.Values(), []int{640}) {
		t.Fatalf("expected width generation request, got %#v", result.PreferredWidths)
	}
}

func TestResolverSelectsFormatVariant(t *testing.T) {
	sourcePath, cat, assetResolver := newResolverFixture(t, "hero.png", "image/png", []byte("origin"), baseAssetsConfig())
	variantPath := filepath.Join(filepath.Dir(sourcePath), "hero.fjpeg.jpg")
	writeTestFile(t, variantPath, []byte("converted"))
	upsertFormatVariant(t, cat, "hero.png", variantPath)

	result, err := assetResolver.Resolve(context.Background(), resolver.Request{Path: "hero.png", Format: "jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Variant == nil || result.Variant.Format != "jpeg" {
		t.Fatalf("expected jpeg format variant, got %#v", result.Variant)
	}
}

func TestResolverRequestsFormatGenerationWhenMissing(t *testing.T) {
	_, _, assetResolver := newResolverFixture(t, "hero.png", "image/png", []byte("origin"), baseAssetsConfig())
	result, err := assetResolver.Resolve(context.Background(), resolver.Request{Path: "hero.png", Format: "jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.PreferredFormats.Values(), []string{"jpeg"}) {
		t.Fatalf("expected format generation request, got %#v", result.PreferredFormats)
	}
}

func TestResolverSelectsFormatVariantFromAccept(t *testing.T) {
	sourcePath, cat, assetResolver := newResolverFixture(t, "hero.png", "image/png", []byte("origin"), baseAssetsConfig())
	variantPath := filepath.Join(filepath.Dir(sourcePath), "hero.fjpeg.jpg")
	writeTestFile(t, variantPath, []byte("converted"))
	upsertFormatVariant(t, cat, "hero.png", variantPath)

	result, err := assetResolver.Resolve(context.Background(), resolver.Request{
		Path:   "hero.png",
		Accept: "image/jpeg,image/png;q=0.5",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Variant == nil || result.Variant.Format != "jpeg" {
		t.Fatalf("expected jpeg format variant, got %#v", result.Variant)
	}
}

func TestResolverRequestsFormatGenerationFromAcceptWhenMissing(t *testing.T) {
	_, _, assetResolver := newResolverFixture(t, "hero.png", "image/png", []byte("origin"), baseAssetsConfig())
	result, err := assetResolver.Resolve(context.Background(), resolver.Request{
		Path:   "hero.png",
		Accept: "image/jpeg,image/png;q=0.5",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, ok := result.PreferredFormats.GetFirst()
	if !ok || first != "jpeg" {
		t.Fatalf("expected format generation request from accept, got %#v", result.PreferredFormats)
	}
}

func TestResolverIgnoresUnsupportedModernFormatsFromAccept(t *testing.T) {
	_, _, assetResolver := newResolverFixture(t, "hero.png", "image/png", []byte("origin"), baseAssetsConfig())
	result, err := assetResolver.Resolve(context.Background(), resolver.Request{
		Path:   "hero.png",
		Accept: "image/webp,image/avif,image/png;q=0.5",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.PreferredFormats.Values(), []string{"png"}) {
		t.Fatalf("expected unsupported modern formats to be ignored, got %#v", result.PreferredFormats)
	}
}

func TestResolverUsesPrefilledPreferredEncodings(t *testing.T) {
	_, _, assetResolver := newResolverFixture(t, "index.html", "text/html; charset=utf-8", []byte("<html>origin</html>"), spaAssetsConfig())
	result, err := assetResolver.Resolve(context.Background(), resolver.Request{
		Path:               "index.html",
		PreferredEncodings: cxlist.NewList[string]("br", "gzip"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.PreferredEncodings == nil {
		t.Fatal("expected preferred encodings to be filled from request")
	}
	if values := result.PreferredEncodings.Values(); !slices.Equal(values, []string{"br", "gzip"}) {
		t.Fatalf("unexpected preferred encodings: %#v", values)
	}
}

func baseAssetsConfig() *config.Assets {
	return &config.Assets{Entry: "index.html"}
}

func spaAssetsConfig() *config.Assets {
	return &config.Assets{
		Entry: "index.html",
		Fallback: config.Fallback{
			On:     config.FallbackOnNotFound,
			Target: "index.html",
		},
	}
}

func newResolverFixture(
	t *testing.T,
	assetPath string,
	mediaType string,
	body []byte,
	cfg *config.Assets,
) (string, catalog.Catalog, *resolver.Resolver) {
	t.Helper()

	sourcePath := filepath.Join(t.TempDir(), assetPath)
	writeTestFile(t, sourcePath, body)

	cat := catalog.NewInMemoryCatalog()
	upsertTestAsset(t, cat, assetPath, sourcePath, mediaType)

	return sourcePath, cat, newTestResolver(cfg, cat)
}

func newTestResolver(cfg *config.Assets, cat catalog.Catalog) *resolver.Resolver {
	return resolver.NewResolverForTest(cfg, cat, slog.New(slog.DiscardHandler))
}

func upsertTestAsset(t *testing.T, cat catalog.Catalog, assetPath, sourcePath, mediaType string) {
	t.Helper()
	if err := cat.UpsertAsset(&catalog.Asset{
		Path:       assetPath,
		FullPath:   sourcePath,
		MediaType:  mediaType,
		SourceHash: testSourceHash,
		ETag:       "\"hash-1\"",
	}); err != nil {
		t.Fatal(err)
	}
}

func upsertFormatVariant(t *testing.T, cat catalog.Catalog, assetPath, variantPath string) {
	t.Helper()
	upsertTestVariant(t, cat, &catalog.Variant{
		ID:           assetPath + "|format=jpeg",
		AssetPath:    assetPath,
		ArtifactPath: variantPath,
		MediaType:    "image/jpeg",
		SourceHash:   testSourceHash,
		ETag:         "\"hash-1-jpeg\"",
		Format:       "jpeg",
		Width:        0,
	})
}

func upsertTestVariant(t *testing.T, cat catalog.Catalog, variant *catalog.Variant) {
	t.Helper()
	if err := cat.UpsertVariant(variant); err != nil {
		t.Fatal(err)
	}
}

func writeTestFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func stringToList(raw string) *cxlist.List[string] {
	return cxlist.NewList(strings.Split(raw, ",")...)
}
