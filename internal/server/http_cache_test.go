package server_test

import (
	"context"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/server"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAssetRouteReturnsNotModifiedForFreshValidator(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "app.js")
	if err := os.WriteFile(assetPath, []byte("console.log('app');"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Root = root

	cat := catalog.NewInMemoryCatalog()
	modifiedAt := time.Unix(1_720_000_000, 0).UTC()
	if err := cat.UpsertAsset(&catalog.Asset{
		Path:       "app.js",
		FullPath:   assetPath,
		Size:       int64(len("console.log('app');")),
		MediaType:  "application/javascript",
		SourceHash: "hash-app",
		ETag:       "\"hash-app\"",
		Metadata: cxmapping.NewMapFrom(map[string]string{
			"mtime_unix": "1720000000",
		}),
	}); err != nil {
		t.Fatal(err)
	}

	app := newHTTPTestApp(
		t,
		&cfg,
		slog.New(slog.DiscardHandler),
		cat,
		assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, slog.New(slog.DiscardHandler)),
		resolver.NewResolverForTest(&cfg.Assets, cat, slog.New(slog.DiscardHandler)),
	)

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/app.js", http.NoBody)
	request.Header.Set("If-None-Match", "\"hash-app\"")
	request.Header.Set("If-Modified-Since", modifiedAt.Format(http.TimeFormat))
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeHTTPBody(t, response)

	if response.StatusCode != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", response.StatusCode)
	}
	if response.Header.Get("ETag") != "\"hash-app\"" {
		t.Fatalf("expected etag header to be preserved, got %q", response.Header.Get("ETag"))
	}
	if response.Header.Get("Last-Modified") != modifiedAt.Format(http.TimeFormat) {
		t.Fatalf("expected last-modified header %q, got %q", modifiedAt.Format(http.TimeFormat), response.Header.Get("Last-Modified"))
	}
	if response.Header.Get("Cache-Control") != "public, max-age=0, must-revalidate" {
		t.Fatalf("expected revalidate cache-control, got %q", response.Header.Get("Cache-Control"))
	}
	assertParseableExpiresHeader(t, response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 0 {
		t.Fatalf("expected empty body for 304, got %q", string(body))
	}
}

func TestVariantRouteSetsCacheHeadersAndHeadHasNoBody(t *testing.T) {
	app := newVariantTestApp(t)

	request := httptest.NewRequestWithContext(context.Background(), http.MethodHead, "/app.js", http.NoBody)
	request.Header.Set("Accept-Encoding", "br")
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeHTTPBody(t, response)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.StatusCode)
	}
	if response.Header.Get("Content-Encoding") != "br" {
		t.Fatalf("expected content-encoding br, got %q", response.Header.Get("Content-Encoding"))
	}
	if response.Header.Get("Cache-Control") != "public, max-age=604800, immutable" {
		t.Fatalf("expected immutable variant cache-control, got %q", response.Header.Get("Cache-Control"))
	}
	if response.Header.Get("Last-Modified") != time.Unix(1_720_000_100, 0).UTC().Format(http.TimeFormat) {
		t.Fatalf("unexpected last-modified header %q", response.Header.Get("Last-Modified"))
	}
	expiresAt := assertParseableExpiresHeader(t, response)
	if expiresAt.Before(time.Now().Add(6 * 24 * time.Hour)) {
		t.Fatalf("expected variant expires to be about a week out, got %s", expiresAt)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 0 {
		t.Fatalf("expected empty body for HEAD, got %q", string(body))
	}
}

func TestVariantRouteFallsBackWhenVariantArtifactIsMissing(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "app.js")
	variantPath := filepath.Join(root, "app.js.br")
	if err := os.WriteFile(assetPath, []byte("console.log('origin');"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Root = root

	cat := catalog.NewInMemoryCatalog()
	upsertAssetForTest(t, cat, &catalog.Asset{
		Path:       "app.js",
		FullPath:   assetPath,
		Size:       int64(len("console.log('origin');")),
		MediaType:  "application/javascript",
		SourceHash: "hash-app",
		ETag:       "\"hash-app\"",
	})
	upsertVariantForTest(t, cat, &catalog.Variant{
		ID:           "app.js|encoding=br",
		AssetPath:    "app.js",
		ArtifactPath: variantPath,
		Size:         2,
		MediaType:    "application/javascript",
		SourceHash:   "hash-app",
		ETag:         "\"hash-app-br\"",
		Encoding:     "br",
	})

	app := newHTTPTestApp(
		t,
		&cfg,
		slog.New(slog.DiscardHandler),
		cat,
		assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, slog.New(slog.DiscardHandler)),
		resolver.NewResolverForTest(&cfg.Assets, cat, slog.New(slog.DiscardHandler)),
	)

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/app.js", http.NoBody)
	request.Header.Set("Accept-Encoding", "br")
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeHTTPBody(t, response)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 fallback response, got %d", response.StatusCode)
	}
	if response.Header.Get("Content-Encoding") != "" {
		t.Fatalf("expected missing variant to fall back to origin asset, got content-encoding %q", response.Header.Get("Content-Encoding"))
	}
	if response.Header.Get("ETag") != "\"hash-app\"" {
		t.Fatalf("expected fallback asset etag, got %q", response.Header.Get("ETag"))
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "console.log('origin');" {
		t.Fatalf("expected origin asset body, got %q", string(body))
	}
	if cat.ListVariants("app.js").Len() != 0 {
		t.Fatalf("expected missing variant to be removed from catalog, got %#v", cat.ListVariants("app.js").Values())
	}
}

func newVariantTestApp(t *testing.T) *fiber.App {
	t.Helper()

	root := t.TempDir()
	assetPath := filepath.Join(root, "app.js")
	variantPath := filepath.Join(root, "app.js.br")
	if err := os.WriteFile(assetPath, []byte("console.log('app');"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(variantPath, []byte("br"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Root = root

	cat := catalog.NewInMemoryCatalog()
	upsertAssetForTest(t, cat, &catalog.Asset{
		Path:       "app.js",
		FullPath:   assetPath,
		Size:       int64(len("console.log('app');")),
		MediaType:  "application/javascript",
		SourceHash: "hash-app",
		ETag:       "\"hash-app\"",
	})
	upsertVariantForTest(t, cat, &catalog.Variant{
		ID:           "app.js|encoding=br",
		AssetPath:    "app.js",
		ArtifactPath: variantPath,
		Size:         2,
		MediaType:    "application/javascript",
		SourceHash:   "hash-app",
		ETag:         "\"hash-app-br\"",
		Encoding:     "br",
		Metadata: cxmapping.NewMapFrom(map[string]string{
			"stage":      "compression",
			"mtime_unix": "1720000100",
		}),
	})

	return newHTTPTestApp(
		t,
		&cfg,
		slog.New(slog.DiscardHandler),
		cat,
		assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, slog.New(slog.DiscardHandler)),
		resolver.NewResolverForTest(&cfg.Assets, cat, slog.New(slog.DiscardHandler)),
	)
}

func newHTTPTestApp(
	t *testing.T,
	cfg *config.Config,
	logger *slog.Logger,
	cat catalog.Catalog,
	bodyCache *assetcache.Cache,
	assetResolver *resolver.Resolver,
) *fiber.App {
	t.Helper()

	app := server.NewAppForTest(cfg, logger, cat, bodyCache, assetResolver, nil)
	t.Cleanup(func() {
		if err := app.Shutdown(); err != nil {
			t.Fatalf("shutdown test app: %v", err)
		}
	})
	return app
}

func upsertAssetForTest(t *testing.T, cat catalog.Catalog, asset *catalog.Asset) {
	t.Helper()
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}
}

func upsertVariantForTest(t *testing.T, cat catalog.Catalog, variant *catalog.Variant) {
	t.Helper()
	if err := cat.UpsertVariant(variant); err != nil {
		t.Fatal(err)
	}
}

func closeHTTPBody(t *testing.T, response *http.Response) {
	t.Helper()
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertParseableExpiresHeader(t *testing.T, response *http.Response) time.Time {
	t.Helper()

	expiresAt, parseErr := http.ParseTime(response.Header.Get("Expires"))
	if parseErr != nil {
		t.Fatalf("expected parseable expires header, got %q: %v", response.Header.Get("Expires"), parseErr)
	}
	return expiresAt
}
