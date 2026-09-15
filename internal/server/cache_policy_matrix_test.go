package server_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/server"
)

type cachePolicyMatrixAsset struct {
	path      string
	mediaType string
	body      string
}

type cachePolicyMatrixCase struct {
	path     string
	encoding string
	method   string
}

var cachePolicyMatrixAssets = [...]cachePolicyMatrixAsset{
	{path: "index.html", mediaType: "text/html; charset=utf-8", body: "<html>app</html>"},
	{path: "assets/app-deadbeef.js", mediaType: "application/javascript", body: "export const app = true;"},
	{path: "assets/style-deadbeef.css", mediaType: "text/css", body: "body { color: red; }"},
	{path: "assets/app.js", mediaType: "application/javascript", body: "export const plain = true;"},
}

func TestHTTPCachePolicyMatrix(t *testing.T) {
	for _, mode := range []string{"direct", "prepared"} {
		t.Run(mode, func(t *testing.T) {
			app := newCachePolicyMatrixApp(t, mode)
			assertMissingCacheMatrixAsset(t, app)
			runCachePolicyMatrixCases(t, app)
		})
	}
}

func cachePolicyMatrixCases() []cachePolicyMatrixCase {
	paths := []string{"/", "/index.html", "/travel/hotel/results", "/?v=old", "/index.html?x=1", "/travel/hotel/results?x=1", "/assets/app-deadbeef.js", "/assets/style-deadbeef.css", "/assets/app.js"}
	encodings := []string{"identity", "br", "gzip", "zstd"}
	methods := []string{http.MethodGet, http.MethodHead}
	cases := make([]cachePolicyMatrixCase, 0, len(paths)*len(encodings)*len(methods))
	for _, path := range paths {
		for _, encoding := range encodings {
			for _, method := range methods {
				cases = append(cases, cachePolicyMatrixCase{path: path, encoding: encoding, method: method})
			}
		}
	}
	return cases
}

func runCachePolicyMatrixCases(t *testing.T, app *fiber.App) {
	t.Helper()
	for _, tc := range cachePolicyMatrixCases() {
		t.Run(tc.method+"/"+tc.path+"/"+tc.encoding, func(t *testing.T) {
			assertHTTPCachePolicyMatrixResponse(t, app, tc)
		})
	}
}

func newCachePolicyMatrixApp(t *testing.T, mode string) *fiber.App {
	t.Helper()
	root := t.TempDir()
	cfg := config.DefaultConfigForTest()
	cfg.Frontend.ImmutableCache.MaxAge = "12h"
	cfg.Assets.Root = root
	cfg.HTTP.MemoryCache.Enable = false
	logger := slog.New(slog.DiscardHandler)
	cat := catalog.NewInMemoryCatalog()
	for _, asset := range cachePolicyMatrixAssets {
		writeCachePolicyMatrixAsset(t, root, cat, asset)
	}
	bodyCache := assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, logger)
	assetResolver := resolver.NewResolverForTest(&cfg.Assets, cat, logger)
	if mode == "direct" {
		return newHTTPTestApp(t, &cfg, logger, cat, bodyCache, assetResolver)
	}
	app, err := server.NewPreparedAppForTest(&cfg, logger, cat, bodyCache, assetResolver, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := app.Shutdown(); err != nil {
			t.Fatalf("shutdown prepared app: %v", err)
		}
	})
	return app
}

func writeCachePolicyMatrixAsset(t *testing.T, root string, cat catalog.Catalog, asset cachePolicyMatrixAsset) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(asset.path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(asset.body), 0o600); err != nil {
		t.Fatal(err)
	}
	sourceHash := "hash-" + asset.path
	upsertAssetForTest(t, cat, &catalog.Asset{
		Path: asset.path, FullPath: fullPath, Size: int64(len(asset.body)),
		MediaType: asset.mediaType, SourceHash: sourceHash, ETag: "\"" + sourceHash + "\"",
	})
	for _, variant := range []struct{ encoding, suffix string }{
		{encoding: "br", suffix: ".br"},
		{encoding: "gzip", suffix: ".gz"},
		{encoding: "zstd", suffix: ".zst"},
	} {
		variantPath := fullPath + variant.suffix
		if err := os.WriteFile(variantPath, []byte(variant.encoding), 0o600); err != nil {
			t.Fatal(err)
		}
		upsertVariantForTest(t, cat, &catalog.Variant{
			ID:        asset.path + "|encoding=" + variant.encoding,
			AssetPath: asset.path, ArtifactPath: variantPath, Size: int64(len(variant.encoding)),
			MediaType: asset.mediaType, SourceHash: sourceHash, Encoding: variant.encoding,
			ETag: "\"" + sourceHash + "-" + variant.encoding + "\"",
		})
	}
}

func assertHTTPCachePolicyMatrixResponse(t *testing.T, app *fiber.App, tc cachePolicyMatrixCase) {
	t.Helper()
	response := sendCachePolicyMatrixRequest(t, app, tc, "")
	defer closeHTTPBody(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", response.StatusCode)
	}
	assertCachePolicyMatrixHeaders(t, response, tc)
	assertCachePolicyMatrixHeadBody(t, response, tc.method)
	assertCachePolicyMatrixNotModified(t, app, tc, response)
	assertCachePolicyMatrixExpires(t, response, tc)
}

func sendCachePolicyMatrixRequest(t *testing.T, app *fiber.App, tc cachePolicyMatrixCase, etag string) *http.Response {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), tc.method, tc.path, http.NoBody)
	request.Header.Set(fiber.HeaderAcceptEncoding, tc.encoding)
	if etag != "" {
		request.Header.Set(fiber.HeaderIfNoneMatch, etag)
	}
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func cachePolicyMatrixPath(path string) string {
	clean, _, _ := strings.Cut(path, "?")
	return clean
}

func cachePolicyMatrixHTML(path string) bool {
	switch cachePolicyMatrixPath(path) {
	case "/", "/index.html", "/travel/hotel/results":
		return true
	default:
		return false
	}
}

func cachePolicyMatrixControl(tc cachePolicyMatrixCase) string {
	if cachePolicyMatrixHTML(tc.path) {
		return "no-cache, max-age=0, must-revalidate"
	}
	if cachePolicyMatrixPath(tc.path) == "/assets/app.js" {
		if tc.encoding == "identity" {
			return "public, max-age=0, must-revalidate"
		}
		return "public, max-age=604800, immutable"
	}
	return "public, max-age=43200, immutable"
}

func assertCachePolicyMatrixHeaders(t *testing.T, response *http.Response, tc cachePolicyMatrixCase) {
	t.Helper()
	wantEncoding := tc.encoding
	if tc.encoding == "identity" {
		wantEncoding = ""
	}
	if got := response.Header.Get(fiber.HeaderContentEncoding); got != wantEncoding {
		t.Fatalf("encoding: got %q, want %q", got, wantEncoding)
	}
	wantControl := cachePolicyMatrixControl(tc)
	if got := response.Header.Get(fiber.HeaderCacheControl); got != wantControl {
		t.Fatalf("cache-control: got %q, want %q", got, wantControl)
	}
}

func assertCachePolicyMatrixHeadBody(t *testing.T, response *http.Response, method string) {
	t.Helper()
	if method != http.MethodHead {
		return
	}
	body, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(body) != 0 {
		t.Fatalf("HEAD returned body %q", body)
	}
}

func assertCachePolicyMatrixNotModified(t *testing.T, app *fiber.App, tc cachePolicyMatrixCase, original *http.Response) {
	t.Helper()
	etag := original.Header.Get(fiber.HeaderETag)
	if etag == "" {
		return
	}
	response := sendCachePolicyMatrixRequest(t, app, tc, etag)
	defer closeHTTPBody(t, response)
	if response.StatusCode != http.StatusNotModified {
		t.Fatalf("conditional status: got %d, want 304", response.StatusCode)
	}
	if got := response.Header.Get(fiber.HeaderCacheControl); got != cachePolicyMatrixControl(tc) {
		t.Fatalf("304 cache-control: got %q", got)
	}
	if got := response.Header.Get(fiber.HeaderETag); got != etag {
		t.Fatalf("304 ETag: got %q, want %q", got, etag)
	}
}

func cachePolicyMatrixExpiryWindow(tc cachePolicyMatrixCase) (time.Duration, time.Duration) {
	if cachePolicyMatrixHTML(tc.path) || (cachePolicyMatrixPath(tc.path) == "/assets/app.js" && tc.encoding == "identity") {
		return -time.Minute, time.Minute
	}
	if cachePolicyMatrixPath(tc.path) == "/assets/app.js" {
		return 6 * 24 * time.Hour, 8 * 24 * time.Hour
	}
	return 11 * time.Hour, 13 * time.Hour
}

func assertCachePolicyMatrixExpires(t *testing.T, response *http.Response, tc cachePolicyMatrixCase) {
	t.Helper()
	expires, parseErr := http.ParseTime(response.Header.Get(fiber.HeaderExpires))
	if parseErr != nil {
		t.Fatalf("expires: %v", parseErr)
	}
	minAge, maxAge := cachePolicyMatrixExpiryWindow(tc)
	now := time.Now()
	if expires.Before(now.Add(minAge)) || expires.After(now.Add(maxAge)) {
		t.Fatalf("expires %s outside window [%s, %s]", expires, minAge, maxAge)
	}
}

func assertMissingCacheMatrixAsset(t *testing.T, app *fiber.App) {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/assets/chunk-deadbeef.js", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeHTTPBody(t, response)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing JS: got %d, want 404", response.StatusCode)
	}
	if strings.Contains(response.Header.Get(fiber.HeaderContentType), "text/html") {
		t.Fatalf("missing JS returned HTML: %q", response.Header.Get(fiber.HeaderContentType))
	}
}
