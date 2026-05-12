package resolver_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
)

type recordedMetric struct {
	name     string
	value    float64
	attrs    map[string]any
	ctxValue any
}

type metricContextKey struct{}

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

func (r recordingCounter) Add(ctx context.Context, value int64, attrs ...observabilityx.Attribute) {
	*r.metrics = append(*r.metrics, recordedMetric{
		name:     r.name,
		value:    float64(value),
		attrs:    attrsToMap(attrs),
		ctxValue: ctx.Value(metricContextKey{}),
	})
}

type recordingHistogram struct {
	name    string
	metrics *[]recordedMetric
}

func (r recordingHistogram) Record(ctx context.Context, value float64, attrs ...observabilityx.Attribute) {
	*r.metrics = append(*r.metrics, recordedMetric{
		name:     r.name,
		value:    value,
		attrs:    attrsToMap(attrs),
		ctxValue: ctx.Value(metricContextKey{}),
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

func TestResolverMetricsRecordFallbackResolution(t *testing.T) {
	obs := &recordingObservability{}
	sourcePath, cat, _ := newResolverFixture(t, "index.html", "text/html; charset=utf-8", []byte("<html>origin</html>"), spaAssetsConfig())
	assetResolver := resolver.NewResolverWithObservabilityForTest(spaAssetsConfig(), cat, slog.New(slog.DiscardHandler), obs)

	ctx := context.WithValue(context.Background(), metricContextKey{}, "resolver-request")
	result, err := assetResolver.Resolve(ctx, resolver.Request{Path: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	if result.FilePath != sourcePath {
		t.Fatalf("expected fallback path %q, got %q", sourcePath, result.FilePath)
	}

	assertCounterMetric(t, obs.counters, "resolver_resolutions_total", "result", "fallback_asset")
	assertHistogramMetric(t, obs.histograms, "resolver_resolution_duration_seconds", "result", "fallback_asset")
	assertMetricContext(t, obs.counters, "resolver_resolutions_total", "resolver-request")
}

func TestResolverMetricsRecordGenerationRequests(t *testing.T) {
	obs := &recordingObservability{}
	sourcePath := writeAssetForMetricTest(t, "hero.png")
	cat := catalog.NewInMemoryCatalog()
	upsertTestAsset(t, cat, "hero.png", sourcePath, "image/png")

	assetResolver := resolver.NewResolverWithObservabilityForTest(baseAssetsConfig(), cat, slog.New(slog.DiscardHandler), obs)
	result, err := assetResolver.Resolve(context.Background(), resolver.Request{Path: "hero.png", Width: 640, Format: "jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Variant != nil {
		t.Fatalf("expected original asset result, got variant %#v", result.Variant)
	}

	assertCounterMetric(t, obs.counters, "resolver_resolutions_total", "result", "asset")
	assertCounterMetric(t, obs.counters, "resolver_generation_requests_total", "kind", "image_width")
	assertCounterMetric(t, obs.counters, "resolver_generation_requests_total", "kind", "image_format")
}

func TestResolverMetricsRecordNotFound(t *testing.T) {
	obs := &recordingObservability{}
	assetResolver := resolver.NewResolverWithObservabilityForTest(&config.Assets{Entry: "index.html"}, catalog.NewInMemoryCatalog(), slog.New(slog.DiscardHandler), obs)

	_, err := assetResolver.Resolve(context.Background(), resolver.Request{Path: "missing.txt"})
	if err == nil {
		t.Fatal("expected not found error")
	}

	assertCounterMetric(t, obs.counters, "resolver_resolutions_total", "result", "not_found")
	assertHistogramMetric(t, obs.histograms, "resolver_resolution_duration_seconds", "result", "not_found")
}

func writeAssetForMetricTest(t *testing.T, assetPath string) string {
	t.Helper()

	sourcePath := testFilePath(t, assetPath)
	writeTestFile(t, sourcePath, []byte("origin"))
	return sourcePath
}

func testFilePath(t *testing.T, assetPath string) string {
	t.Helper()
	return t.TempDir() + "/" + assetPath
}

func attrsToMap(attrs []observabilityx.Attribute) map[string]any {
	values := make(map[string]any, len(attrs))
	for _, attr := range attrs {
		values[attr.Key] = attr.Value
	}
	return values
}

func assertCounterMetric(t *testing.T, metrics []recordedMetric, name, key string, want any) {
	t.Helper()

	for _, metric := range metrics {
		if metric.name != name || metric.value != 1 {
			continue
		}
		if got := metric.attrs[key]; got == want {
			return
		}
	}
	t.Fatalf("expected counter %s with %s=%v", name, key, want)
}

func assertHistogramMetric(t *testing.T, metrics []recordedMetric, name, key string, want any) {
	t.Helper()

	for _, metric := range metrics {
		if metric.name != name {
			continue
		}
		if got := metric.attrs[key]; got == want {
			return
		}
	}
	t.Fatalf("expected histogram %s with %s=%v", name, key, want)
}

func assertMetricContext(t *testing.T, metrics []recordedMetric, name string, want any) {
	t.Helper()

	for _, metric := range metrics {
		if metric.name == name && metric.ctxValue == want {
			return
		}
	}
	t.Fatalf("expected metric %s with context value %v", name, want)
}
