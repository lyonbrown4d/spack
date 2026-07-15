package server_test

import (
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRobotsRouteGeneratesConfiguredContent(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Root = t.TempDir()
	cfg.Robots.Enable = true
	cfg.Robots.Override = false
	cfg.Robots.Allow = ""
	cfg.Robots.Disallow = "/admin"
	cfg.Robots.Sitemap = "https://example.com/sitemap.xml"

	cat := catalog.NewInMemoryCatalog()
	app := newHTTPTestApp(
		t,
		&cfg,
		slog.New(slog.DiscardHandler),
		cat,
		assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, slog.New(slog.DiscardHandler)),
		resolver.NewResolverForTest(&cfg.Assets, cat, slog.New(slog.DiscardHandler)),
	)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/robots.txt", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeHTTPBody(t, response)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.StatusCode)
	}
	if response.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("unexpected content-type %q", response.Header.Get("Content-Type"))
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	expected := "User-agent: *\nDisallow: /admin\nSitemap: https://example.com/sitemap.xml\n"
	if string(body) != expected {
		t.Fatalf("expected generated robots body %q, got %q", expected, string(body))
	}
}

func TestRobotsRoutePrefersStaticAssetWhenAvailable(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "robots.txt")
	payload := "User-agent: *\nDisallow: /\n"
	if err := os.WriteFile(assetPath, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Root = root
	cfg.Assets.Path = "/static"
	cfg.Robots.Enable = true
	cfg.Robots.Override = false
	cfg.Robots.Disallow = "/generated"

	cat := catalog.NewInMemoryCatalog()
	if err := cat.UpsertAsset(&catalog.Asset{
		Path:       "robots.txt",
		FullPath:   assetPath,
		Size:       int64(len(payload)),
		MediaType:  "text/plain; charset=utf-8",
		SourceHash: "hash-robots",
		ETag:       "\"hash-robots\"",
		Metadata: cxmapping.NewMapFrom(map[string]string{
			"mtime_unix": "1720000200",
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

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/robots.txt", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeHTTPBody(t, response)

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != payload {
		t.Fatalf("expected static robots body %q, got %q", payload, string(body))
	}
}
