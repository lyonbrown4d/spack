package server_test

import (
	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/server"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetricsMiddlewareRecordsAssetDeliveryMetrics(t *testing.T) {
	obs := &recordingObservability{}
	app := fiber.New()
	app.Use(server.MetricsMiddlewareForTest(obs))
	app.Get("/", func(c fiber.Ctx) error {
		server.SetAssetDeliveryForTest(c, "memory_cache_hit")
		return c.SendStatus(fiber.StatusNoContent)
	})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeBody(t, response)

	assertMetricCount(t, obs.counters, "http_requests_total", 1)
	assertMetricCount(t, obs.histograms, "http_request_duration_seconds", 1)
	assertMetricCount(t, obs.counters, "http_asset_delivery_total", 1)
	assertMetricCount(t, obs.histograms, "http_asset_delivery_duration_seconds", 1)

	requestAttrs := map[string]any{
		"method": http.MethodGet,
		"route":  "/",
		"status": "204",
	}
	deliveryAttrs := map[string]any{
		"method":   http.MethodGet,
		"route":    "/",
		"status":   "204",
		"delivery": "memory_cache_hit",
	}
	assertMetricContract(t, findMetric(t, obs.counters, "http_requests_total"), requestAttrs)
	assertMetricContract(t, findMetric(t, obs.histograms, "http_request_duration_seconds"), requestAttrs)
	assertMetricContract(t, findMetric(t, obs.counters, "http_asset_delivery_total"), deliveryAttrs)
	assertMetricContract(t, findMetric(t, obs.histograms, "http_asset_delivery_duration_seconds"), deliveryAttrs)
}

func TestMetricsMiddlewareUsesFinalErrorStatusForAssetDelivery(t *testing.T) {
	obs := &recordingObservability{}
	app := fiber.New()
	app.Use(server.MetricsMiddlewareForTest(obs))
	app.Get("/asset", func(c fiber.Ctx) error {
		server.SetAssetDeliveryForTest(c, "source")
		return fiber.NewError(fiber.StatusRequestedRangeNotSatisfiable, "sensitive range detail")
	})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/asset", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeBody(t, response)

	requestCounter := findMetric(t, obs.counters, "http_requests_total")
	assertAttrValue(t, requestCounter.attrs, "status", "416")
	deliveryCounter := findMetric(t, obs.counters, "http_asset_delivery_total")
	assertAttrValue(t, deliveryCounter.attrs, "status", "416")
}

func TestMetricsMiddlewareMapsUnsafeFiberStatusToInternalError(t *testing.T) {
	obs := &recordingObservability{}
	app := fiber.New()
	app.Use(server.MetricsMiddlewareForTest(obs))
	app.Get("/unauthorized", func(c fiber.Ctx) error {
		server.SetAssetDeliveryForTest(c, "source")
		return fiber.NewError(fiber.StatusUnauthorized, "sensitive auth detail")
	})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/unauthorized", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeBody(t, response)

	requestAttrs := map[string]any{
		"method": http.MethodGet,
		"route":  "/unauthorized",
		"status": "500",
	}
	deliveryAttrs := map[string]any{
		"method":   http.MethodGet,
		"route":    "/unauthorized",
		"status":   "500",
		"delivery": "source",
	}
	assertMetricContract(t, findMetric(t, obs.counters, "http_requests_total"), requestAttrs)
	assertMetricContract(t, findMetric(t, obs.histograms, "http_request_duration_seconds"), requestAttrs)
	assertMetricContract(t, findMetric(t, obs.counters, "http_asset_delivery_total"), deliveryAttrs)
	assertMetricContract(t, findMetric(t, obs.histograms, "http_asset_delivery_duration_seconds"), deliveryAttrs)
}

func TestMetricsMiddlewareSkipsAssetDeliveryMetricsWithoutDelivery(t *testing.T) {
	obs := &recordingObservability{}
	app := fiber.New()
	app.Use(server.MetricsMiddlewareForTest(obs))
	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeBody(t, response)

	assertMetricCount(t, obs.counters, "http_requests_total", 1)
	assertMetricCount(t, obs.histograms, "http_request_duration_seconds", 1)
	assertMetricCount(t, obs.counters, "http_asset_delivery_total", 0)
	assertMetricCount(t, obs.histograms, "http_asset_delivery_duration_seconds", 0)

	requestCounter := findMetric(t, obs.counters, "http_requests_total")
	assertAttrValue(t, requestCounter.attrs, "route", "/healthz")
}

func TestMetricsMiddlewareTracksInFlightRequests(t *testing.T) {
	obs := &recordingObservability{}
	runtimeMetrics := server.NewRuntimeMetrics()
	app := fiber.New()
	app.Use(server.MetricsMiddlewareWithRuntimeMetricsForTest(obs, runtimeMetrics))
	app.Get("/", func(c fiber.Ctx) error {
		if got := testutil.ToFloat64(runtimeMetrics.RequestsInFlight); got != 1 {
			t.Fatalf("expected in-flight gauge to be 1 during request, got %v", got)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeBody(t, response)

	if got := testutil.ToFloat64(runtimeMetrics.RequestsInFlight); got != 0 {
		t.Fatalf("expected in-flight gauge to return to 0, got %v", got)
	}
}
