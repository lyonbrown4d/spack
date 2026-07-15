package task_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/task"
)

type recordedMetric struct {
	name  string
	value float64
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

func (r recordingCounter) Add(_ context.Context, value int64, attrs ...observabilityx.Attribute) {
	*r.metrics = append(*r.metrics, recordedMetric{
		name:  r.name,
		value: float64(value),
		attrs: attrsToMap(attrs),
	})
}

type recordingHistogram struct {
	name    string
	metrics *[]recordedMetric
}

func (r recordingHistogram) Record(_ context.Context, value float64, attrs ...observabilityx.Attribute) {
	*r.metrics = append(*r.metrics, recordedMetric{
		name:  r.name,
		value: value,
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

func TestRecordTaskRunMetrics(t *testing.T) {
	obs := &recordingObservability{}

	task.RecordTaskRunMetricsForTest(t.Context(), obs, "source_rescan", time.Now().Add(-time.Second), nil)

	assertCounterMetric(t, obs.counters, "task_runs_total", 1, "task", "source_rescan")
	assertCounterMetric(t, obs.counters, "task_runs_total", 1, "result", "ok")
	assertHistogramMetric(t, obs.histograms, "task_run_duration_seconds", "task", "source_rescan")
}

func TestRecordSourceRescanMetrics(t *testing.T) {
	obs := &recordingObservability{}

	task.RecordSourceRescanMetricsForTest(t.Context(), obs, task.SourceRescanReport{
		TotalBytes:         4096,
		Scanned:            10,
		Added:              2,
		Updated:            1,
		Removed:            3,
		RemovedVariants:    4,
		RemovedArtifacts:   5,
		CacheInvalidations: 6,
	})

	assertCounterMetric(t, obs.counters, "source_rescan_scanned_bytes_total", 4096, "", nil)
	assertCounterMetric(t, obs.counters, "source_rescan_scanned_total", 10, "", nil)
	assertCounterMetric(t, obs.counters, "source_rescan_added_total", 2, "", nil)
	assertCounterMetric(t, obs.counters, "source_rescan_updated_total", 1, "", nil)
	assertCounterMetric(t, obs.counters, "source_rescan_removed_total", 3, "", nil)
	assertCounterMetric(t, obs.counters, "source_rescan_removed_variants_total", 4, "", nil)
	assertCounterMetric(t, obs.counters, "source_rescan_removed_artifacts_total", 5, "", nil)
	assertCounterMetric(t, obs.counters, "source_rescan_cache_invalidations_total", 6, "", nil)
}

func TestRecordArtifactJanitorAndCacheWarmerMetrics(t *testing.T) {
	obs := &recordingObservability{}

	task.RecordArtifactJanitorMetricsForTest(t.Context(), obs, task.ArtifactJanitorReport{
		ScannedArtifacts:   12,
		RemovedOrphans:     2,
		RemovedDirectories: 1,
		MissingVariants:    3,
		CacheInvalidations: 4,
	})
	task.RecordCacheWarmerMetricsForTest(t.Context(), obs, task.CacheWarmerReport{
		Assets:        3,
		Variants:      5,
		LoadedEntries: 7,
		LoadedBytes:   1024,
	})

	assertCounterMetric(t, obs.counters, "artifact_janitor_scanned_artifacts_total", 12, "", nil)
	assertCounterMetric(t, obs.counters, "artifact_janitor_removed_orphans_total", 2, "", nil)
	assertCounterMetric(t, obs.counters, "artifact_janitor_removed_directories_total", 1, "", nil)
	assertCounterMetric(t, obs.counters, "artifact_janitor_missing_variants_total", 3, "", nil)
	assertCounterMetric(t, obs.counters, "artifact_janitor_cache_invalidations_total", 4, "", nil)
	assertCounterMetric(t, obs.counters, "cache_warmer_assets_total", 3, "", nil)
	assertCounterMetric(t, obs.counters, "cache_warmer_variants_total", 5, "", nil)
	assertCounterMetric(t, obs.counters, "cache_warmer_loaded_entries_total", 7, "", nil)
	assertCounterMetric(t, obs.counters, "cache_warmer_loaded_bytes_total", 1024, "", nil)
}

func attrsToMap(attrs []observabilityx.Attribute) map[string]any {
	values := make(map[string]any, len(attrs))
	for _, attr := range attrs {
		values[attr.Key] = attr.Value
	}
	return values
}

func assertCounterMetric(t *testing.T, metrics []recordedMetric, name string, wantValue float64, key string, want any) {
	t.Helper()

	for _, metric := range metrics {
		if metric.name != name || metric.value != wantValue {
			continue
		}
		if key == "" || metric.attrs[key] == want {
			return
		}
	}
	t.Fatalf("expected counter %s with value %v", name, wantValue)
}

func assertHistogramMetric(t *testing.T, metrics []recordedMetric, name, key string, want any) {
	t.Helper()

	for _, metric := range metrics {
		if metric.name != name {
			continue
		}
		if metric.attrs[key] == want {
			return
		}
	}
	t.Fatalf("expected histogram %s with %s=%v", name, key, want)
}
