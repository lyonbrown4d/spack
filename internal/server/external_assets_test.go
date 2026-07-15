package server_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/server"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/task"
)

const externalAssetsRootEnv = "SPACK_TEST_ASSETS_ROOT"

func TestEscapedUnicodeJavaScriptAssetFromExternalBuild(t *testing.T) {
	root := strings.TrimSpace(os.Getenv(externalAssetsRootEnv))
	if root == "" {
		t.Skipf("%s is not set", externalAssetsRootEnv)
	}

	logger := slog.New(slog.DiscardHandler)
	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Root = root

	src, err := source.NewLocalFS(&cfg.Assets, logger)
	if err != nil {
		t.Fatal(err)
	}

	cat := catalog.NewInMemoryCatalog()
	report, err := task.SyncSourceCatalogForTest(t.Context(), src, cat, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Scanned == 0 {
		t.Fatal("expected scanned assets from external build root")
	}

	assetPath, ok := firstUnicodeJavaScriptAssetPath(cat)
	if !ok {
		t.Fatal("expected at least one unicode javascript asset in external build root")
	}

	app := server.NewAppForTest(
		&cfg,
		logger,
		cat,
		assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, logger),
		resolver.NewResolverForTest(&cfg.Assets, cat, logger),
		nil,
	)
	t.Cleanup(func() {
		if shutdownErr := app.Shutdown(); shutdownErr != nil {
			t.Fatalf("shutdown test app: %v", shutdownErr)
		}
	})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/"+escapeURLPath(assetPath),
		http.NoBody,
	)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeHTTPBody(t, response)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for escaped unicode asset path %q, got %d", assetPath, response.StatusCode)
	}
	if got := response.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/javascript") {
		t.Fatalf("expected javascript content-type for %q, got %q", assetPath, got)
	}
}

func firstUnicodeJavaScriptAssetPath(cat catalog.Catalog) (string, bool) {
	var picked string
	cat.AllAssets().Range(func(_ int, asset *catalog.Asset) bool {
		if asset == nil || !strings.HasSuffix(asset.Path, ".js") || !containsNonASCII(asset.Path) {
			return true
		}
		picked = asset.Path
		return false
	})
	return picked, picked != ""
}

func containsNonASCII(value string) bool {
	return strings.IndexFunc(value, func(char rune) bool {
		return char > 127
	}) >= 0
}
