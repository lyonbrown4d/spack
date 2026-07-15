package server_test

import (
	"bytes"

	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
)

func TestMissingAssetPathDoesNotFallbackToHTML(t *testing.T) {
	root := t.TempDir()
	indexPath := filepath.Join(root, "index.html")
	if err := os.WriteFile(indexPath, []byte("<html>app</html>"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Root = root

	cat := catalog.NewInMemoryCatalog()
	upsertAssetForTest(t, cat, &catalog.Asset{
		Path:       "index.html",
		FullPath:   indexPath,
		Size:       int64(len("<html>app</html>")),
		MediaType:  "text/html; charset=utf-8",
		SourceHash: "hash-index",
		ETag:       "\"hash-index\"",
	})

	app := newHTTPTestApp(
		t,
		&cfg,
		slog.New(slog.DiscardHandler),
		cat,
		assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, slog.New(slog.DiscardHandler)),
		resolver.NewResolverForTest(&cfg.Assets, cat, slog.New(slog.DiscardHandler)),
	)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/assets/index-missing.js", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeHTTPBody(t, response)

	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing asset path, got %d", response.StatusCode)
	}
	if contentType := response.Header.Get("Content-Type"); strings.Contains(contentType, "text/html") {
		t.Fatalf("expected missing asset path not to respond with html content-type, got %q", contentType)
	}
}

func TestAssetMountPathRequiresSegmentBoundary(t *testing.T) {
	root := t.TempDir()
	indexPayload := []byte("<html>app</html>")
	appPayload := []byte("console.log('app');")
	indexPath := filepath.Join(root, "index.html")
	appPath := filepath.Join(root, "app.js")
	if err := os.WriteFile(indexPath, indexPayload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(appPath, appPayload, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Path = "/assets"
	cfg.Assets.Root = root

	cat := catalog.NewInMemoryCatalog()
	upsertAssetForTest(t, cat, &catalog.Asset{
		Path:       "index.html",
		FullPath:   indexPath,
		Size:       int64(len(indexPayload)),
		MediaType:  "text/html; charset=utf-8",
		SourceHash: "hash-index",
		ETag:       "\"hash-index\"",
	})
	upsertAssetForTest(t, cat, &catalog.Asset{
		Path:       "app.js",
		FullPath:   appPath,
		Size:       int64(len(appPayload)),
		MediaType:  "application/javascript",
		SourceHash: "hash-app",
		ETag:       "\"hash-app\"",
	})

	app := newHTTPTestApp(
		t,
		&cfg,
		slog.New(slog.DiscardHandler),
		cat,
		assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, slog.New(slog.DiscardHandler)),
		resolver.NewResolverForTest(&cfg.Assets, cat, slog.New(slog.DiscardHandler)),
	)

	for _, tc := range []struct {
		path string
		want int
	}{
		{path: "/assets", want: http.StatusOK},
		{path: "/assets/app.js", want: http.StatusOK},
		{path: "/assetsx/app.js", want: http.StatusNotFound},
		{path: "/assets2/app.js", want: http.StatusNotFound},
		{path: "/assets-admin/app.js", want: http.StatusNotFound},
	} {
		t.Run(tc.path, func(t *testing.T) {
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tc.path, http.NoBody)
			response, err := app.Test(request)
			if err != nil {
				t.Fatal(err)
			}
			defer closeHTTPBody(t, response)

			if response.StatusCode != tc.want {
				t.Fatalf("expected status %d for %s, got %d", tc.want, tc.path, response.StatusCode)
			}
		})
	}
}

func TestUnicodeAssetPathResolvesFromEscapedURL(t *testing.T) {
	root := t.TempDir()
	assetName := "我的订单_inactive.js"
	assetPath := filepath.Join(root, assetName)
	payload := []byte("console.log('unicode chunk');")
	if err := os.WriteFile(assetPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Root = root

	cat := catalog.NewInMemoryCatalog()
	upsertAssetForTest(t, cat, &catalog.Asset{
		Path:       assetName,
		FullPath:   assetPath,
		Size:       int64(len(payload)),
		MediaType:  "application/javascript",
		SourceHash: "hash-unicode",
		ETag:       "\"hash-unicode\"",
	})

	app := newHTTPTestApp(
		t,
		&cfg,
		slog.New(slog.DiscardHandler),
		cat,
		assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, slog.New(slog.DiscardHandler)),
		resolver.NewResolverForTest(&cfg.Assets, cat, slog.New(slog.DiscardHandler)),
	)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/"+escapeURLPath(assetName),
		http.NoBody,
	)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeHTTPBody(t, response)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for escaped unicode asset path, got %d", response.StatusCode)
	}
	if got := response.Header.Get("Content-Type"); got != "application/javascript" {
		t.Fatalf("expected application/javascript, got %q", got)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, payload) {
		t.Fatalf("expected payload %q, got %q", string(payload), string(body))
	}
}

func escapeURLPath(value string) string {
	segments := strings.Split(value, "/")
	for index, segment := range segments {
		segments[index] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
