package server_test

import (
	"bytes"
	"compress/gzip"

	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/lyonbrown4d/spack/internal/task"
)

type sidecarBoundaryFixture struct {
	root         string
	bundlePath   string
	png          []byte
	validGzip    []byte
	explicitGzip []byte
}

func TestAssetRouteSidecarTrustBoundariesInDirectAndBundleModes(t *testing.T) {
	fixture := newSidecarBoundaryFixture(t)

	for _, tc := range []struct {
		name string
		root string
	}{
		{name: "direct", root: fixture.root},
		{name: "bundle", root: fixture.bundlePath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newSidecarBoundaryApp(t, tc.root)

			assertSidecarBoundaryEncodedResponse(t, app, "/assets/hero.png", fixture.validGzip)
			assertSidecarBoundaryOriginFallback(t, app, "/assets/bad.png", fixture.png)
			assertSidecarBoundaryMissing(t, app, "/assets/bad.png.gz")
		})
	}
}

func TestAssetRouteServesExplicitBundleCompressionVariant(t *testing.T) {
	fixture := newSidecarBoundaryFixture(t)
	app := newSidecarBoundaryApp(t, fixture.bundlePath)

	assertSidecarBoundaryEncodedResponse(t, app, "/assets/explicit.js", fixture.explicitGzip)
}

func newSidecarBoundaryFixture(t *testing.T) sidecarBoundaryFixture {
	t.Helper()

	root := t.TempDir()
	png := validPNGForServerBoundaryTest(t)
	validGzip := gzipPayloadForServerBoundaryTest(t, png)
	explicitSource := []byte("console.log('explicit');")
	explicitGzip := gzipPayloadForServerBoundaryTest(t, explicitSource)

	writeSidecarBoundarySourceFiles(t, root, png, validGzip, explicitSource, explicitGzip)
	return sidecarBoundaryFixture{
		root:         root,
		bundlePath:   writeSidecarBoundaryBundle(t, root),
		png:          png,
		validGzip:    validGzip,
		explicitGzip: explicitGzip,
	}
}

func writeSidecarBoundarySourceFiles(
	t *testing.T,
	root string,
	png []byte,
	validGzip []byte,
	explicitSource []byte,
	explicitGzip []byte,
) {
	t.Helper()
	writeServerBoundaryFile(t, filepath.Join(root, "assets", "hero.png"), png)
	writeServerBoundaryFile(t, filepath.Join(root, "assets", "hero.png.gz"), validGzip)
	writeServerBoundaryFile(t, filepath.Join(root, "assets", "bad.png"), png)
	writeServerBoundaryFile(t, filepath.Join(root, "assets", "bad.png.gz"), gzipPayloadForServerBoundaryTest(t, []byte("not a png")))
	writeServerBoundaryFile(t, filepath.Join(root, "assets", "explicit.js"), explicitSource)
	writeServerBoundaryFile(t, filepath.Join(root, "generated", "compression", "assets", "explicit.js.encoding-gzip.gz"), explicitGzip)
}

func writeSidecarBoundaryBundle(t *testing.T, root string) string {
	t.Helper()
	bundlePath := filepath.Join(t.TempDir(), "app.spack")
	if _, err := spackbundle.Write(t.Context(), spackbundle.WriteOptions{
		Output: bundlePath,
		Root:   root,
		Files:  sidecarBoundaryBundleFiles(root),
	}); err != nil {
		t.Fatal(err)
	}
	return bundlePath
}

func sidecarBoundaryBundleFiles(root string) []spackbundle.File {
	return []spackbundle.File{
		sidecarBoundaryAssetFile(root, "assets/hero.png", "image/png", "hash-hero"),
		sidecarBoundarySourceSidecarFile(root, "assets/hero.png.gz", "assets/hero.png", "hash-hero"),
		sidecarBoundaryAssetFile(root, "assets/bad.png", "image/png", "hash-bad"),
		sidecarBoundarySourceSidecarFile(root, "assets/bad.png.gz", "assets/bad.png", "hash-bad"),
		sidecarBoundaryAssetFile(root, "assets/explicit.js", "application/javascript", "hash-explicit"),
		sidecarBoundaryExplicitCompressionFile(root),
	}
}

func sidecarBoundaryAssetFile(root, path, mediaType, hash string) spackbundle.File {
	return spackbundle.File{
		Path:       path,
		FullPath:   filepath.Join(root, filepath.FromSlash(path)),
		Kind:       "asset",
		MediaType:  mediaType,
		SourceHash: hash,
		ETag:       `"` + hash + `"`,
	}
}

func sidecarBoundarySourceSidecarFile(root, path, assetPath, hash string) spackbundle.File {
	return spackbundle.File{
		Path:       path,
		FullPath:   filepath.Join(root, filepath.FromSlash(path)),
		Kind:       sourcecatalog.SourceSidecarStage,
		MediaType:  "image/png",
		SourceHash: hash,
		ETag:       `"` + hash + `-gzip"`,
		AssetPath:  assetPath,
		Encoding:   "gzip",
	}
}

func sidecarBoundaryExplicitCompressionFile(root string) spackbundle.File {
	path := "generated/compression/assets/explicit.js.encoding-gzip.gz"
	return spackbundle.File{
		Path:       path,
		FullPath:   filepath.Join(root, filepath.FromSlash(path)),
		Kind:       sourcecatalog.BundleCompressionStage,
		MediaType:  "application/javascript",
		SourceHash: "hash-explicit",
		ETag:       `"hash-explicit-gzip"`,
		AssetPath:  "assets/explicit.js",
		Encoding:   "gzip",
	}
}

func newSidecarBoundaryApp(t *testing.T, root string) *fiber.App {
	t.Helper()

	logger := slog.New(slog.DiscardHandler)
	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Root = root

	src, err := source.NewLocalFS(&cfg.Assets, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cleanupErr := src.Cleanup(); cleanupErr != nil {
			t.Fatal(cleanupErr)
		}
	})

	cat := catalog.NewInMemoryCatalog()
	if _, err := task.SyncSourceCatalogForTest(t.Context(), src, cat, nil); err != nil {
		t.Fatal(err)
	}

	return newHTTPTestApp(
		t,
		&cfg,
		logger,
		cat,
		assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, logger),
		resolver.NewResolverForTest(&cfg.Assets, cat, logger),
	)
}

func assertSidecarBoundaryEncodedResponse(t *testing.T, app *fiber.App, path string, want []byte) {
	t.Helper()

	response := sendSidecarBoundaryRequest(t, app, path, "gzip")
	defer closeHTTPBody(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for %s, got %d", path, response.StatusCode)
	}
	if response.Header.Get(fiber.HeaderContentEncoding) != "gzip" {
		t.Fatalf("expected gzip content-encoding for %s, got %q", path, response.Header.Get(fiber.HeaderContentEncoding))
	}
	body := readSidecarBoundaryBody(t, response)
	if !bytes.Equal(body, want) {
		t.Fatalf("expected encoded body for %s to match trusted sidecar", path)
	}
}

func assertSidecarBoundaryOriginFallback(t *testing.T, app *fiber.App, path string, want []byte) {
	t.Helper()

	response := sendSidecarBoundaryRequest(t, app, path, "gzip")
	defer closeHTTPBody(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 fallback for %s, got %d", path, response.StatusCode)
	}
	if response.Header.Get(fiber.HeaderContentEncoding) != "" {
		t.Fatalf("expected origin fallback without content-encoding for %s, got %q", path, response.Header.Get(fiber.HeaderContentEncoding))
	}
	body := readSidecarBoundaryBody(t, response)
	if !bytes.Equal(body, want) {
		t.Fatalf("expected origin body for %s", path)
	}
}

func assertSidecarBoundaryMissing(t *testing.T, app *fiber.App, path string) {
	t.Helper()

	response := sendSidecarBoundaryRequest(t, app, path, "gzip")
	defer closeHTTPBody(t, response)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected invalid sidecar path %s to be hidden with 404, got %d", path, response.StatusCode)
	}
}

func sendSidecarBoundaryRequest(t *testing.T, app *fiber.App, path, acceptEncoding string) *http.Response {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody)
	request.Header.Set(fiber.HeaderAcceptEncoding, acceptEncoding)
	request.Header.Set(fiber.HeaderAccept, "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func readSidecarBoundaryBody(t *testing.T, response *http.Response) []byte {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func writeServerBoundaryFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func gzipPayloadForServerBoundaryTest(t *testing.T, body []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := gzip.NewWriter(&out)
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func validPNGForServerBoundaryTest(t *testing.T) []byte {
	t.Helper()
	body, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAAXNSR0IArs4c6QAAAARnQU1BAACxjwv8YQUAAAAJcEhZcwAADsMAAA7DAcdvqGQAAAANSURBVBhXY/jPwPAfAAUAAf+mXJtdAAAAAElFTkSuQmCC")
	if err != nil {
		t.Fatal(err)
	}
	return body
}
