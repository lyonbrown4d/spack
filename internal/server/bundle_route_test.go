package server_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/lyonbrown4d/spack/internal/task"
)

func TestBundleAssetRouteServesRangeFromSourceBundle(t *testing.T) {
	app := newBundleRouteTestApp(t)
	response := sendBundleRangeRequest(t, app)
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	assertBundleRangeResponse(t, response)
}

func newBundleRouteTestApp(t *testing.T) *fiber.App {
	t.Helper()

	root, assetPath := writeBundleRouteAsset(t)
	bundlePath := writeBundleRouteBundle(t, root, assetPath)
	logger := slog.New(slog.DiscardHandler)
	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Root = bundlePath

	cat := scanBundleRouteCatalog(t, &cfg, logger)
	assertBundleCatalogAsset(t, cat)

	return newHTTPTestApp(
		t,
		&cfg,
		logger,
		cat,
		assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, logger),
		resolver.NewResolverForTest(&cfg.Assets, cat, logger),
	)
}

func writeBundleRouteAsset(t *testing.T) (string, string) {
	t.Helper()

	root := t.TempDir()
	assetPath := filepath.Join(root, "assets", "app.js")
	if err := os.MkdirAll(filepath.Dir(assetPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(assetPath, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, assetPath
}

func writeBundleRouteBundle(t *testing.T, root, assetPath string) string {
	t.Helper()

	bundlePath := filepath.Join(t.TempDir(), "app.spack")
	if _, err := spackbundle.Write(context.Background(), spackbundle.WriteOptions{
		Output: bundlePath,
		Root:   root,
		Files: []spackbundle.File{
			{
				Path:       "assets/app.js",
				FullPath:   assetPath,
				Kind:       "asset",
				MediaType:  "application/javascript",
				SourceHash: "hash-app",
				ETag:       `"hash-app"`,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return bundlePath
}

func scanBundleRouteCatalog(t *testing.T, cfg *config.Config, logger *slog.Logger) catalog.Catalog {
	t.Helper()

	src, err := source.NewLocalFS(&cfg.Assets, logger)
	if err != nil {
		t.Fatal(err)
	}
	cat := catalog.NewInMemoryCatalog()
	if _, syncErr := task.SyncSourceCatalogForTest(context.Background(), src, cat, nil); syncErr != nil {
		t.Fatal(syncErr)
	}
	return cat
}

func assertBundleCatalogAsset(t *testing.T, cat catalog.Catalog) {
	t.Helper()

	asset, ok := cat.FindAsset("assets/app.js")
	if !ok || asset == nil {
		t.Fatal("expected bundle asset in catalog")
	}
	if !spackbundle.IsReference(asset.FullPath) {
		t.Fatalf("expected bundle reference full path, got %q", asset.FullPath)
	}
	if asset.ETag != `"hash-app"` {
		t.Fatalf("expected bundle etag, got %q", asset.ETag)
	}
}

func sendBundleRangeRequest(t *testing.T, app *fiber.App) *http.Response {
	t.Helper()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/assets/app.js", http.NoBody)
	request.Header.Set(fiber.HeaderRange, "bytes=2-5")
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func assertBundleRangeResponse(t *testing.T, response *http.Response) {
	t.Helper()

	if response.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d", response.StatusCode)
	}
	if response.Header.Get(fiber.HeaderContentRange) != "bytes 2-5/10" {
		t.Fatalf("expected content-range bytes 2-5/10, got %q", response.Header.Get(fiber.HeaderContentRange))
	}
	if response.Header.Get(fiber.HeaderContentLength) != "4" {
		t.Fatalf("expected content-length 4, got %q", response.Header.Get(fiber.HeaderContentLength))
	}
	if response.Header.Get(fiber.HeaderETag) != `"hash-app"` {
		t.Fatalf("expected etag header, got %q", response.Header.Get(fiber.HeaderETag))
	}
	if contentType := response.Header.Get(fiber.HeaderContentType); !strings.HasPrefix(contentType, "application/javascript") {
		t.Fatalf("expected javascript content-type, got %q", contentType)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "2345" {
		t.Fatalf("expected range body 2345, got %q", string(body))
	}
}
