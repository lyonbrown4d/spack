package server_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/requestpath"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/server"
)

const (
	preparedBenchmarkAssetCount       = 1_000
	preparedBenchmarkVariantCount     = 2
	preparedBenchmarkRoutePath        = "asset-500.js"
	preparedBenchmarkAcceptEncoding   = "gzip, br"
	preparedBenchmarkSmallFilePayload = "console.log('prepared');"
)

func BenchmarkPreparedSnapshotBuild(b *testing.B) {
	cfg := newPreparedBenchmarkConfig()
	logger := slog.New(slog.DiscardHandler)
	cat := newPreparedBenchmarkCatalog(b, &cfg, preparedBenchmarkAssetCount, preparedBenchmarkVariantCount)
	svc := server.NewPreparedServiceForTest(&cfg, logger, cat)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if err := svc.Rebuild(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPreparedResolveEncoding(b *testing.B) {
	cfg := newPreparedBenchmarkConfig()
	svc := newPreparedBenchmarkService(b, &cfg)
	request := resolver.Request{
		Path:           preparedBenchmarkRoutePath,
		AcceptEncoding: preparedBenchmarkAcceptEncoding,
	}
	cleanedPath := requestpath.Clean(request.Path)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		selection, ok := server.ResolvePreparedCleanedForTest(svc, request, "", cleanedPath)
		if !ok || selection.FilePath == "" {
			b.Fatal("expected prepared selection")
		}
	}
}

func BenchmarkPreparedResourceHintsCacheHit(b *testing.B) {
	cfg := newPreparedBenchmarkConfig()
	logger := slog.New(slog.DiscardHandler)
	root := b.TempDir()
	htmlPath := filepath.Join(root, "index.html")
	body := []byte(`<!doctype html><link rel="stylesheet" href="/assets/app.css"><script type="module" src="/assets/app.js"></script>`)
	if err := os.WriteFile(htmlPath, body, 0o600); err != nil {
		b.Fatal(err)
	}

	cfg.Assets.Root = root
	service := server.NewResourceHintServiceForTest(&cfg, logger)
	result := &resolver.Result{
		Asset: &catalog.Asset{
			Path:      "index.html",
			FullPath:  htmlPath,
			MediaType: "text/html; charset=utf-8",
		},
	}
	if _, ok := service.Entry(result); !ok {
		b.Fatal("expected resource hints cache warmup")
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		entry, ok := service.Entry(result)
		if !ok || entry.Header == "" {
			b.Fatal("expected cached resource hints")
		}
	}
}

func BenchmarkHTTPPreparedSmallFile(b *testing.B) {
	cfg := newPreparedBenchmarkConfig()
	root := b.TempDir()
	assetPath := filepath.Join(root, "app.js")
	payload := []byte(preparedBenchmarkSmallFilePayload)
	if err := os.WriteFile(assetPath, payload, 0o600); err != nil {
		b.Fatal(err)
	}

	cfg.Assets.Root = root
	cat := catalog.NewInMemoryCatalog()
	if err := cat.UpsertAsset(&catalog.Asset{
		Path:       "app.js",
		FullPath:   assetPath,
		Size:       int64(len(payload)),
		MediaType:  "application/javascript",
		SourceHash: "bench-prepared",
		ETag:       "\"bench-prepared\"",
	}); err != nil {
		b.Fatal(err)
	}

	assetResolver := resolver.NewResolverForTest(&cfg.Assets, cat, slog.New(slog.DiscardHandler))
	app, err := server.NewPreparedAppForTest(&cfg, slog.New(slog.DiscardHandler), cat, nil, assetResolver, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := app.Shutdown(); err != nil {
			b.Fatalf("shutdown benchmark app: %v", err)
		}
	})

	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()

	for range b.N {
		runPreparedHTTPBenchmarkIteration(b, app, "/app.js")
	}
}

func newPreparedBenchmarkConfig() config.Config {
	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.HTTP.MemoryCache.Enable = true
	cfg.HTTP.MemoryCache.MaxFileSize = 64 * 1024
	cfg.HTTP.MemoryCache.MaxEntries = preparedBenchmarkAssetCount * (preparedBenchmarkVariantCount + 1)
	cfg.Frontend.ResourceHints.Enable = true
	cfg.Frontend.ResourceHints.EarlyHints = false
	cfg.Frontend.ResourceHints.MaxLinks = 8
	cfg.Frontend.ResourceHints.MaxHeaderBytes = 2048
	return cfg
}

func newPreparedBenchmarkService(b *testing.B, cfg *config.Config) *server.PreparedService {
	b.Helper()

	svc := server.NewPreparedServiceForTest(
		cfg,
		slog.New(slog.DiscardHandler),
		newPreparedBenchmarkCatalog(b, cfg, preparedBenchmarkAssetCount, preparedBenchmarkVariantCount),
	)
	if err := svc.Rebuild(context.Background()); err != nil {
		b.Fatal(err)
	}
	return svc
}

func newPreparedBenchmarkCatalog(
	b *testing.B,
	cfg *config.Config,
	assetCount int,
	variantsPerAsset int,
) catalog.Catalog {
	b.Helper()

	root := b.TempDir()
	cfg.Assets.Root = root
	artifactPath := filepath.Join(root, "artifact.js")
	body := []byte(preparedBenchmarkSmallFilePayload)
	if err := os.WriteFile(artifactPath, body, 0o600); err != nil {
		b.Fatal(err)
	}

	cat := catalog.NewInMemoryCatalog()
	for index := range assetCount {
		assetPath := "asset-" + strconv.Itoa(index) + ".js"
		upsertPreparedBenchmarkAsset(b, cat, assetPath, artifactPath, int64(len(body)))
		for variantIndex := range variantsPerAsset {
			upsertPreparedBenchmarkVariant(b, cat, assetPath, artifactPath, variantIndex)
		}
	}
	return cat
}

func upsertPreparedBenchmarkAsset(
	b *testing.B,
	cat catalog.Catalog,
	assetPath string,
	fullPath string,
	size int64,
) {
	b.Helper()

	if err := cat.UpsertAsset(&catalog.Asset{
		Path:       assetPath,
		FullPath:   fullPath,
		Size:       size,
		MediaType:  "application/javascript",
		SourceHash: "hash-" + assetPath,
		ETag:       "\"hash-" + assetPath + "\"",
	}); err != nil {
		b.Fatal(err)
	}
}

func upsertPreparedBenchmarkVariant(
	b *testing.B,
	cat catalog.Catalog,
	assetPath string,
	artifactPath string,
	variantIndex int,
) {
	b.Helper()

	encoding := "gzip"
	if variantIndex%2 == 0 {
		encoding = "br"
	}
	if err := cat.UpsertVariant(&catalog.Variant{
		ID:           assetPath + "|encoding=" + encoding,
		AssetPath:    assetPath,
		ArtifactPath: artifactPath,
		Size:         int64(len(preparedBenchmarkSmallFilePayload)),
		MediaType:    "application/javascript",
		SourceHash:   "hash-" + assetPath,
		ETag:         "\"hash-" + assetPath + "-" + encoding + "\"",
		Encoding:     encoding,
	}); err != nil {
		b.Fatal(err)
	}
}

func runPreparedHTTPBenchmarkIteration(b *testing.B, app *fiber.App, requestURL string) {
	b.Helper()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, requestURL, http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		if response != nil {
			closePreparedBenchmarkBody(b, response.Body)
		}
		b.Fatal(err)
	}
	defer closePreparedBenchmarkBody(b, response.Body)

	if response.StatusCode != http.StatusOK {
		b.Fatalf("expected 200, got %d", response.StatusCode)
	}
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		b.Fatal(err)
	}
}

func closePreparedBenchmarkBody(b *testing.B, body io.Closer) {
	b.Helper()
	if err := body.Close(); err != nil {
		b.Fatal(err)
	}
}
