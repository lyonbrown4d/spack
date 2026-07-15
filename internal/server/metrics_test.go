package server_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arcgolabs/observabilityx"
	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/server"
	"github.com/prometheus/client_golang/prometheus/testutil"
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

	requestCounter := findMetric(t, obs.counters, "http_requests_total")
	assertAttrAbsent(t, requestCounter.attrs, "delivery")
	assertAttrValue(t, requestCounter.attrs, "route", "/")

	deliveryCounter := findMetric(t, obs.counters, "http_asset_delivery_total")
	assertAttrValue(t, deliveryCounter.attrs, "delivery", "memory_cache_hit")
	assertAttrValue(t, deliveryCounter.attrs, "route", "/")
	assertAttrValue(t, deliveryCounter.attrs, "status", "204")
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

type recordedMetric struct {
	name  string
	attrs map[string]any
}

type recordingObservability struct {
	counters   []recordedMetric
	histograms []recordedMetric
}

func (r *recordingObservability) Logger() *slog.Logger {
	return slog.Default()
}

func (r *recordingObservability) StartSpan(
	ctx context.Context,
	_ string,
	_ ...observabilityx.Attribute,
) (context.Context, observabilityx.Span) {
	return ctx, recordingSpan{}
}

func (r *recordingObservability) Counter(spec observabilityx.CounterSpec) observabilityx.Counter {
	return recordingCounter{name: spec.Name, metrics: &r.counters}
}

func (r *recordingObservability) UpDownCounter(observabilityx.UpDownCounterSpec) observabilityx.UpDownCounter {
	return noopUpDownCounter{}
}

func (r *recordingObservability) Histogram(spec observabilityx.HistogramSpec) observabilityx.Histogram {
	return recordingHistogram{name: spec.Name, metrics: &r.histograms}
}

func (r *recordingObservability) Gauge(observabilityx.GaugeSpec) observabilityx.Gauge {
	return noopGauge{}
}

type recordingCounter struct {
	name    string
	metrics *[]recordedMetric
}

func (r recordingCounter) Add(_ context.Context, _ int64, attrs ...observabilityx.Attribute) {
	*r.metrics = append(*r.metrics, recordedMetric{
		name:  r.name,
		attrs: attrsToMap(attrs),
	})
}

type recordingHistogram struct {
	name    string
	metrics *[]recordedMetric
}

func (r recordingHistogram) Record(_ context.Context, _ float64, attrs ...observabilityx.Attribute) {
	*r.metrics = append(*r.metrics, recordedMetric{
		name:  r.name,
		attrs: attrsToMap(attrs),
	})
}

type noopUpDownCounter struct{}

func (noopUpDownCounter) Add(context.Context, int64, ...observabilityx.Attribute) {}

type noopGauge struct{}

func (noopGauge) Set(context.Context, float64, ...observabilityx.Attribute) {}

type recordingSpan struct{}

func (recordingSpan) End() {}

func (recordingSpan) RecordError(error) {}

func (recordingSpan) SetAttributes(...observabilityx.Attribute) {}

func attrsToMap(attrs []observabilityx.Attribute) map[string]any {
	values := make(map[string]any, len(attrs))
	for _, attr := range attrs {
		values[attr.Key] = attr.Value
	}
	return values
}

func assertMetricCount(t *testing.T, metrics []recordedMetric, name string, want int) {
	t.Helper()

	count := 0
	for _, metric := range metrics {
		if metric.name == name {
			count++
		}
	}
	if count != want {
		t.Fatalf("expected %d %s metrics, got %d", want, name, count)
	}
}

func findMetric(t *testing.T, metrics []recordedMetric, name string) recordedMetric {
	t.Helper()

	for _, metric := range metrics {
		if metric.name == name {
			return metric
		}
	}
	t.Fatalf("metric %s not found", name)
	return recordedMetric{}
}

func assertAttrValue(t *testing.T, attrs map[string]any, key string, want any) {
	t.Helper()

	got, ok := attrs[key]
	if !ok {
		t.Fatalf("expected attr %s to be present", key)
	}
	if got != want {
		t.Fatalf("expected attr %s=%v, got %v", key, want, got)
	}
}

func assertAttrAbsent(t *testing.T, attrs map[string]any, key string) {
	t.Helper()

	if _, ok := attrs[key]; ok {
		t.Fatalf("expected attr %s to be absent", key)
	}
}

func closeBody(t *testing.T, response *http.Response) {
	t.Helper()

	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
}
