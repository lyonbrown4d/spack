package server_test

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"testing"

	"github.com/arcgolabs/observabilityx"
)

type recordedMetric struct {
	name      string
	labelKeys []string
	attrs     map[string]any
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
	return recordingCounter{
		name:      spec.Name,
		labelKeys: spec.LabelKeys.Values(),
		metrics:   &r.counters,
	}
}

func (r *recordingObservability) UpDownCounter(observabilityx.UpDownCounterSpec) observabilityx.UpDownCounter {
	return noopUpDownCounter{}
}

func (r *recordingObservability) Histogram(spec observabilityx.HistogramSpec) observabilityx.Histogram {
	return recordingHistogram{
		name:      spec.Name,
		labelKeys: spec.LabelKeys.Values(),
		metrics:   &r.histograms,
	}
}

func (r *recordingObservability) Gauge(observabilityx.GaugeSpec) observabilityx.Gauge {
	return noopGauge{}
}

type recordingCounter struct {
	name      string
	labelKeys []string
	metrics   *[]recordedMetric
}

func (r recordingCounter) Add(_ context.Context, _ int64, attrs ...observabilityx.Attribute) {
	*r.metrics = append(*r.metrics, recordedMetric{
		name:      r.name,
		labelKeys: slices.Clone(r.labelKeys),
		attrs:     attrsToMap(attrs),
	})
}

type recordingHistogram struct {
	name      string
	labelKeys []string
	metrics   *[]recordedMetric
}

func (r recordingHistogram) Record(_ context.Context, _ float64, attrs ...observabilityx.Attribute) {
	*r.metrics = append(*r.metrics, recordedMetric{
		name:      r.name,
		labelKeys: slices.Clone(r.labelKeys),
		attrs:     attrsToMap(attrs),
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

func assertMetricContract(t *testing.T, metric recordedMetric, wantAttrs map[string]any) {
	t.Helper()

	wantKeys := make([]string, 0, len(wantAttrs))
	for key := range wantAttrs {
		wantKeys = append(wantKeys, key)
	}
	slices.Sort(wantKeys)

	gotLabelKeys := slices.Clone(metric.labelKeys)
	slices.Sort(gotLabelKeys)
	if !slices.Equal(gotLabelKeys, wantKeys) {
		t.Fatalf("metric %s label keys mismatch: got %v, want %v", metric.name, gotLabelKeys, wantKeys)
	}

	gotAttrKeys := make([]string, 0, len(metric.attrs))
	for key := range metric.attrs {
		gotAttrKeys = append(gotAttrKeys, key)
	}
	slices.Sort(gotAttrKeys)
	if !slices.Equal(gotAttrKeys, wantKeys) {
		t.Fatalf("metric %s attr keys mismatch: got %v, want %v", metric.name, gotAttrKeys, wantKeys)
	}
	for key, want := range wantAttrs {
		if got := metric.attrs[key]; got != want {
			t.Fatalf("metric %s attr %s mismatch: got %v, want %v", metric.name, key, got, want)
		}
	}
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

func closeBody(t *testing.T, response *http.Response) {
	t.Helper()

	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
}
