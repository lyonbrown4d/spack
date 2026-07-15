package server_test

import (
	"bytes"

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
	"github.com/lyonbrown4d/spack/internal/server"
)

func TestAssetRouteReusesHotResponseEntry(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "app.js")
	payload := []byte("console.log('hot');")
	if err := os.WriteFile(assetPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Root = root

	cat := catalog.NewInMemoryCatalog()
	upsertAssetForTest(t, cat, &catalog.Asset{
		Path:       "app.js",
		FullPath:   assetPath,
		Size:       int64(len(payload)),
		MediaType:  "application/javascript",
		SourceHash: "hash-app",
		ETag:       "\"hash-app\"",
	})

	obs := &recordingObservability{}
	logger := slog.New(slog.DiscardHandler)
	app := server.NewObservedAppForTest(
		&cfg,
		logger,
		obs,
		nil,
		cat,
		assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, logger),
		resolver.NewResolverForTest(&cfg.Assets, cat, logger),
		nil,
	)
	t.Cleanup(func() {
		if err := app.Shutdown(); err != nil {
			t.Fatalf("shutdown test app: %v", err)
		}
	})

	requestAssetForHotResponse(t, app, payload)
	requestAssetForHotResponse(t, app, payload)

	assertDeliveryMetric(t, obs, "memory_cache_fill")
	assertDeliveryMetric(t, obs, "hot_response_hit")
}

func requestAssetForHotResponse(t *testing.T, app *fiber.App, want []byte) {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/app.js", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	closeHTTPBody(t, response)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.StatusCode)
	}
	if !bytes.Equal(body, want) {
		t.Fatalf("expected payload %q, got %q", want, body)
	}
}

func assertDeliveryMetric(t *testing.T, obs *recordingObservability, delivery string) {
	t.Helper()

	for _, metric := range obs.counters {
		if metric.name == "http_asset_delivery_total" && metric.attrs["delivery"] == delivery {
			return
		}
	}
	t.Fatalf("expected asset delivery metric %q, got %#v", delivery, obs.counters)
}
