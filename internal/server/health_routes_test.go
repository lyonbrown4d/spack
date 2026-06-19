package server_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/server"
)

type healthResponse struct {
	Kind    string             `json:"kind"`
	Healthy bool               `json:"healthy"`
	Checks  map[string]*string `json:"checks"`
}

func TestHealthRoutesReturnHealthyReports(t *testing.T) {
	root := t.TempDir()

	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Root = root

	cat := catalog.NewInMemoryCatalog()
	app := newHTTPTestApp(
		t,
		&cfg,
		slog.New(slog.DiscardHandler),
		cat,
		assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, slog.New(slog.DiscardHandler)),
		resolver.NewResolverForTest(&cfg.Assets, cat, slog.New(slog.DiscardHandler)),
	)

	assertHealthResponse(t, app, "/healthz", http.StatusOK, "general", "catalog", "")
	assertHealthResponse(t, app, "/livez", http.StatusOK, "liveness", "server", "")
	assertHealthResponse(t, app, "/readyz", http.StatusOK, "readiness", "assets_root", "")
}

func TestCatalogRouteRequiresDebugEnabled(t *testing.T) {
	root := t.TempDir()
	cat := catalog.NewInMemoryCatalog()

	disabledCfg := config.DefaultConfigForTest()
	disabledCfg.Debug.Enable = false
	disabledCfg.Assets.Root = root
	disabledApp := newHTTPTestApp(
		t,
		&disabledCfg,
		slog.New(slog.DiscardHandler),
		cat,
		assetcache.NewCacheForTest(disabledCfg.HTTP.MemoryCache, slog.New(slog.DiscardHandler)),
		resolver.NewResolverForTest(&disabledCfg.Assets, cat, slog.New(slog.DiscardHandler)),
	)
	assertCatalogRouteStatus(t, disabledApp, http.StatusNotFound)

	enabledCfg := config.DefaultConfigForTest()
	enabledCfg.Debug.Enable = true
	enabledCfg.Assets.Root = root
	enabledApp := newHTTPTestApp(
		t,
		&enabledCfg,
		slog.New(slog.DiscardHandler),
		cat,
		assetcache.NewCacheForTest(enabledCfg.HTTP.MemoryCache, slog.New(slog.DiscardHandler)),
		resolver.NewResolverForTest(&enabledCfg.Assets, cat, slog.New(slog.DiscardHandler)),
	)
	assertCatalogRouteStatus(t, enabledApp, http.StatusOK)
}

func TestReadinessRouteReturnsUnavailableWhenAssetsRootIsMissing(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Root = t.TempDir() + "/missing"

	cat := catalog.NewInMemoryCatalog()
	app := newHTTPTestApp(
		t,
		&cfg,
		slog.New(slog.DiscardHandler),
		cat,
		assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, slog.New(slog.DiscardHandler)),
		resolver.NewResolverForTest(&cfg.Assets, cat, slog.New(slog.DiscardHandler)),
	)

	assertHealthResponse(t, app, "/readyz", http.StatusServiceUnavailable, "readiness", "assets_root", "stat assets root")
}

func TestHealthRoutesRecordRuntimeMetrics(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Root = t.TempDir() + "/missing"

	cat := catalog.NewInMemoryCatalog()
	obs := &recordingObservability{}
	app := server.NewObservedAppForTest(
		&cfg,
		slog.New(slog.DiscardHandler),
		obs,
		nil,
		cat,
		assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, slog.New(slog.DiscardHandler)),
		resolver.NewResolverForTest(&cfg.Assets, cat, slog.New(slog.DiscardHandler)),
		nil,
		nil,
	)
	t.Cleanup(func() {
		if err := app.Shutdown(); err != nil {
			t.Fatalf("shutdown test app: %v", err)
		}
	})

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeHTTPBody(t, response)

	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected readiness route to return 503, got %d", response.StatusCode)
	}

	assertMetricPresent(t, obs.counters, "health_check_runs_total", map[string]any{
		"kind":   "readiness",
		"check":  "assets_root",
		"result": "error",
	})
	assertMetricPresent(t, obs.histograms, "health_check_duration_seconds", map[string]any{
		"kind":   "readiness",
		"check":  "assets_root",
		"result": "error",
	})
	assertMetricPresent(t, obs.counters, "health_reports_total", map[string]any{
		"kind":   "readiness",
		"result": "error",
	})
	assertMetricPresent(t, obs.histograms, "health_report_duration_seconds", map[string]any{
		"kind":   "readiness",
		"result": "error",
	})
}

func assertCatalogRouteStatus(t *testing.T, app *fiber.App, status int) {
	t.Helper()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/catalog", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeHTTPBody(t, response)

	if response.StatusCode != status {
		t.Fatalf("expected /catalog to return %d, got %d", status, response.StatusCode)
	}
	if status != http.StatusOK {
		return
	}

	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
}

func assertHealthResponse(
	t *testing.T,
	app *fiber.App,
	path string,
	status int,
	kind string,
	checkName string,
	checkContains string,
) {
	t.Helper()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeHTTPBody(t, response)

	if response.StatusCode != status {
		t.Fatalf("expected %s to return %d, got %d", path, status, response.StatusCode)
	}

	var payload healthResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Kind != kind {
		t.Fatalf("expected %s kind %q, got %q", path, kind, payload.Kind)
	}

	checkMessage, ok := payload.Checks[checkName]
	if !ok {
		t.Fatalf("expected %s check %q to be present", path, checkName)
	}
	if checkContains == "" {
		if checkMessage != nil {
			t.Fatalf("expected %s check %q to be healthy, got %q", path, checkName, *checkMessage)
		}
		return
	}
	if checkMessage == nil || !strings.Contains(*checkMessage, checkContains) {
		t.Fatalf("expected %s check %q to contain %q, got %v", path, checkName, checkContains, checkMessage)
	}
}

func assertMetricPresent(t *testing.T, metrics []recordedMetric, name string, want map[string]any) {
	t.Helper()

	for _, metric := range metrics {
		if metric.name != name {
			continue
		}
		matched := true
		for key, value := range want {
			if metric.attrs[key] != value {
				matched = false
				break
			}
		}
		if matched {
			return
		}
	}
	t.Fatalf("metric %s with attrs %v not found", name, want)
}
