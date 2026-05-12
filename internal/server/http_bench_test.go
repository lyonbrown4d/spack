package server_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/server"
)

func BenchmarkHTTPAssetRouteMemoryCacheHit(b *testing.B) {
	const payload = "console.log('app');"

	app := newHTTPBenchmarkApp(b, true, "app.js", []byte(payload))
	requestURL := "/app.js"

	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()

	runHTTPRouteBenchmark(b, app, requestURL)
}

func BenchmarkHTTPAssetRouteSendfile(b *testing.B) {
	payload := make([]byte, 128*1024)

	app := newHTTPBenchmarkApp(b, false, "bundle.js", payload)
	requestURL := "/bundle.js"

	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()

	runHTTPRouteBenchmark(b, app, requestURL)
}

func runHTTPRouteBenchmark(b *testing.B, app *fiber.App, requestURL string) {
	b.Helper()

	for range b.N {
		runHTTPRouteBenchmarkIteration(b, app, requestURL)
	}
}

func runHTTPRouteBenchmarkIteration(b *testing.B, app *fiber.App, requestURL string) {
	b.Helper()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, requestURL, http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		if response != nil {
			closeBenchmarkBody(b, response.Body)
		}
		b.Fatal(err)
	}
	defer closeBenchmarkBody(b, response.Body)

	if response.StatusCode != http.StatusOK {
		b.Fatalf("expected 200, got %d", response.StatusCode)
	}
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		b.Fatal(err)
	}
}

func newHTTPBenchmarkApp(b *testing.B, memoryCacheEnabled bool, assetName string, payload []byte) *fiber.App {
	b.Helper()

	//nolint:usetesting // Windows sendfile may still hold the file during benchmark cleanup.
	root, err := os.MkdirTemp("", "spack-http-bench-*")
	if err != nil {
		b.Fatal(err)
	}
	assetPath := filepath.Join(root, assetName)
	if err := os.WriteFile(assetPath, payload, 0o600); err != nil {
		b.Fatal(err)
	}

	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Root = root
	cfg.HTTP.MemoryCache.Enable = memoryCacheEnabled
	cfg.HTTP.MemoryCache.MaxFileSize = int64(len(payload)) + 1024
	cfg.HTTP.MemoryCache.MaxEntries = 32

	cat := catalog.NewInMemoryCatalog()
	if err := cat.UpsertAsset(&catalog.Asset{
		Path:       assetName,
		FullPath:   assetPath,
		Size:       int64(len(payload)),
		MediaType:  "application/javascript",
		SourceHash: "bench-hash",
		ETag:       "\"bench-hash\"",
	}); err != nil {
		b.Fatal(err)
	}

	logger := slog.New(slog.DiscardHandler)
	bodyCache := assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, logger)
	if memoryCacheEnabled {
		if _, found, err := bodyCache.GetOrLoad(assetPath); err != nil {
			b.Fatal(err)
		} else if found {
			b.Fatal("expected benchmark cache warmup to miss on first load")
		}
	}

	assetResolver := resolver.NewResolverForTest(&cfg.Assets, cat, logger)
	app := server.NewAppForTest(&cfg, logger, cat, bodyCache, assetResolver, nil, nil)
	b.Cleanup(func() {
		if err := app.Shutdown(); err != nil {
			b.Fatalf("shutdown benchmark app: %v", err)
		}
		removeBenchmarkDir(b, root)
	})
	return app
}

func closeBenchmarkBody(b *testing.B, body io.Closer) {
	b.Helper()
	if err := body.Close(); err != nil {
		b.Fatal(err)
	}
}

func removeBenchmarkDir(b *testing.B, root string) {
	b.Helper()

	var lastErr error
	for range 40 {
		lastErr = os.RemoveAll(root)
		if lastErr == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	b.Logf("benchmark temp dir cleanup deferred for %s: %v", root, lastErr)
}
