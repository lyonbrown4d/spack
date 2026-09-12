package server

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

type testFiberApp interface {
	Test(*http.Request, ...fiber.TestConfig) (*http.Response, error)
}

func compilePreparedSendFileResponse(tb testing.TB, src *source.LocalFS, file source.File) *preparedResponse {
	tb.Helper()

	cfg := config.DefaultConfigForTest()
	cfg.HTTP.MemoryCache.Enable = false
	compiler := newPreparedCompiler(
		&cfg,
		nil,
		slog.New(slog.DiscardHandler),
		newServerFileSourcesFromSource(src),
	)
	return compiler.compileAssetResponse(&catalog.Asset{
		Path:      file.Path,
		FullPath:  file.FullPath,
		Size:      file.Size,
		MediaType: "application/javascript; charset=utf-8",
		ETag:      "\"trusted-etag\"",
	})
}

func newPreparedMapFSResponse(t *testing.T, body string, modTime time.Time) *preparedResponse {
	t.Helper()

	fullPath := filepath.Join(t.TempDir(), "app.js")
	if err := os.WriteFile(fullPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if !modTime.IsZero() {
		if err := os.Chtimes(fullPath, modTime, modTime); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.DefaultConfigForTest()
	cfg.HTTP.MemoryCache.Enable = false
	compiler := newPreparedCompiler(&cfg, nil, slog.New(slog.DiscardHandler), nil)
	response := compiler.compileResponse(resolver.Result{
		Asset: &catalog.Asset{
			Path:      "app.js",
			FullPath:  fullPath,
			Size:      int64(len(body)),
			MediaType: "application/javascript; charset=utf-8",
			ETag:      "\"trusted-etag\"",
		},
		FilePath:        fullPath,
		MediaType:       "application/javascript; charset=utf-8",
		ETag:            "\"trusted-etag\"",
		ContentEncoding: "br",
	}, "")
	response.sendFile = &preparedSendFile{
		root: fstest.MapFS{
			"app.js": &fstest.MapFile{Data: []byte(body), ModTime: modTime},
		},
		path: "app.js",
	}
	return response
}

func newPreparedResponseApp(response *preparedResponse, runtime *assetDeliveryRuntime) *fiber.App {
	if runtime == nil {
		runtime = &assetDeliveryRuntime{}
	}
	app := fiber.New()
	app.Add([]string{fiber.MethodGet, fiber.MethodHead}, "/app.js", func(c fiber.Ctx) error {
		request := resolver.Request{
			Path:           "app.js",
			RangeRequested: c.Get(fiber.HeaderRange) != "",
		}
		_, _, err := runtime.sendPreparedAsset(c, request, preparedSelection{response: response})
		return err
	})
	return app
}

func sendPreparedTestRequest(t *testing.T, app testFiberApp, request *http.Request) *http.Response {
	t.Helper()

	result, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func readPreparedResponseBody(t *testing.T, response *http.Response) string {
	t.Helper()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func closePreparedResponse(t *testing.T, response *http.Response) {
	t.Helper()

	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertPreparedSendFileHeaders(t *testing.T, response *http.Response, modTime time.Time) {
	t.Helper()

	wants := map[string]string{
		fiber.HeaderContentType:     "application/javascript; charset=utf-8",
		fiber.HeaderContentLength:   "19",
		fiber.HeaderContentEncoding: "br",
		fiber.HeaderETag:            "\"trusted-etag\"",
		fiber.HeaderLastModified:    modTime.Format(http.TimeFormat),
	}
	for key, want := range wants {
		if got := response.Header.Get(key); got != want {
			t.Fatalf("expected %s %q, got %q", key, want, got)
		}
	}
	if got := response.Header.Get(fiber.HeaderCacheControl); got == "" {
		t.Fatal("expected cache-control header")
	}
}

func newPreparedSendFileDirectSource(tb testing.TB, body []byte) (*source.LocalFS, source.File) {
	tb.Helper()

	root := tb.TempDir()
	fullPath := filepath.Join(root, "app.js")
	if err := os.WriteFile(fullPath, body, 0o600); err != nil {
		tb.Fatal(err)
	}
	src, ok, err := source.NewLocalDirectory(root)
	if err != nil {
		tb.Fatal(err)
	}
	if !ok {
		tb.Fatal("expected direct local source")
	}
	tb.Cleanup(func() {
		if cleanupErr := src.Cleanup(); cleanupErr != nil {
			tb.Fatal(cleanupErr)
		}
	})
	info, err := os.Stat(fullPath)
	if err != nil {
		tb.Fatal(err)
	}
	return src, source.File{
		Path:     "app.js",
		FullPath: fullPath,
		Size:     info.Size(),
		ModTime:  info.ModTime(),
	}
}

func newPreparedSendFileBundleSource(tb testing.TB, body []byte) (*source.LocalFS, source.File) {
	tb.Helper()

	root := tb.TempDir()
	fullPath := filepath.Join(root, "app.js")
	if err := os.WriteFile(fullPath, body, 0o600); err != nil {
		tb.Fatal(err)
	}
	bundlePath := filepath.Join(tb.TempDir(), "app.spack")
	if _, err := spackbundle.Write(tb.Context(), spackbundle.WriteOptions{
		Output: bundlePath,
		Root:   root,
		Files: []spackbundle.File{
			{
				Path:      "app.js",
				FullPath:  fullPath,
				Kind:      "asset",
				MediaType: "application/javascript; charset=utf-8",
				ETag:      "\"trusted-etag\"",
			},
		},
	}); err != nil {
		tb.Fatal(err)
	}
	cfg := config.DefaultConfigForTest().Assets
	cfg.Root = bundlePath
	src, err := source.NewLocalFS(&cfg, slog.New(slog.DiscardHandler))
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if cleanupErr := src.Cleanup(); cleanupErr != nil {
			tb.Fatal(cleanupErr)
		}
	})
	file, found, err := src.FindFile("app.js")
	if err != nil {
		tb.Fatal(err)
	}
	if !found {
		tb.Fatal("expected extracted bundle file")
	}
	return src, file
}
