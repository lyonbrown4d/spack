package pipeline_test

import (
	"context"
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/pipeline"
	"log/slog"
	"testing"
)

type pipelineMetric struct {
	name  string
	value float64
	attrs map[string]any
}

type pipelineRecordingObservability struct {
	counters   []pipelineMetric
	histograms []pipelineMetric
}

func (r *pipelineRecordingObservability) Logger() *slog.Logger {
	return slog.Default()
}

func (r *pipelineRecordingObservability) StartSpan(
	ctx context.Context,
	_ string,
	_ ...observabilityx.Attribute,
) (context.Context, observabilityx.Span) {
	return ctx, pipelineRecordingSpan{}
}

func (r *pipelineRecordingObservability) Counter(spec observabilityx.CounterSpec) observabilityx.Counter {
	return pipelineRecordingCounter{name: spec.Name, metrics: &r.counters}
}

func (r *pipelineRecordingObservability) UpDownCounter(observabilityx.UpDownCounterSpec) observabilityx.UpDownCounter {
	return pipelineNoopUpDownCounter{}
}

func (r *pipelineRecordingObservability) Histogram(spec observabilityx.HistogramSpec) observabilityx.Histogram {
	return pipelineRecordingHistogram{name: spec.Name, metrics: &r.histograms}
}

func (r *pipelineRecordingObservability) Gauge(observabilityx.GaugeSpec) observabilityx.Gauge {
	return pipelineNoopGauge{}
}

type pipelineRecordingCounter struct {
	name    string
	metrics *[]pipelineMetric
}

func (r pipelineRecordingCounter) Add(_ context.Context, value int64, attrs ...observabilityx.Attribute) {
	*r.metrics = append(*r.metrics, pipelineMetric{
		name:  r.name,
		value: float64(value),
		attrs: pipelineAttrsToMap(attrs),
	})
}

type pipelineRecordingHistogram struct {
	name    string
	metrics *[]pipelineMetric
}

func (r pipelineRecordingHistogram) Record(_ context.Context, value float64, attrs ...observabilityx.Attribute) {
	*r.metrics = append(*r.metrics, pipelineMetric{
		name:  r.name,
		value: value,
		attrs: pipelineAttrsToMap(attrs),
	})
}

type pipelineNoopUpDownCounter struct{}

func (pipelineNoopUpDownCounter) Add(context.Context, int64, ...observabilityx.Attribute) {}

type pipelineNoopGauge struct{}

func (pipelineNoopGauge) Set(context.Context, float64, ...observabilityx.Attribute) {}

type pipelineRecordingSpan struct{}

func (pipelineRecordingSpan) End() {}

func (pipelineRecordingSpan) RecordError(error) {}

func (pipelineRecordingSpan) SetAttributes(...observabilityx.Attribute) {}

type stageResultStage struct {
	name    string
	variant *catalog.Variant
	err     error
}

func (s stageResultStage) Name() string {
	return s.name
}

func (stageResultStage) Plan(_ *catalog.Asset, _ pipeline.Request) *cxlist.List[pipeline.Task] {
	return nil
}

func (s stageResultStage) Execute(_ pipeline.Task, _ *catalog.Asset) (*catalog.Variant, error) {
	return s.variant, s.err
}

func TestPipelineStageMetricsRecordRunAndGeneration(t *testing.T) {
	obs := &pipelineRecordingObservability{}
	cat := catalog.NewInMemoryCatalog()
	asset := &catalog.Asset{
		Path:       "app.js",
		FullPath:   "/tmp/app.js",
		MediaType:  "application/javascript",
		SourceHash: "hash-1",
		ETag:       "\"hash-1\"",
	}
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}

	svc := pipeline.NewServiceWithObservabilityForTest(&config.Compression{}, slog.New(slog.DiscardHandler), cat, obs, 1)
	variant := &catalog.Variant{
		ID:           "app.js|encoding=br",
		AssetPath:    "app.js",
		ArtifactPath: "/tmp/app.js.br",
		Size:         123,
		MediaType:    "application/javascript",
		SourceHash:   "hash-1",
		ETag:         "\"hash-1-br\"",
		Encoding:     "br",
	}

	got := pipeline.ExecuteStageTaskForTest(svc, stageResultStage{name: "compression", variant: variant}, asset, pipeline.Task{AssetPath: "app.js", Encoding: "br"})
	if got == nil {
		t.Fatal("expected variant result")
	}
	pipeline.UpsertStageVariantForTest(svc, "compression", asset, variant)

	assertPipelineCounterMetric(t, obs.counters, "pipeline_stage_runs_total", 1, "stage", "compression")
	assertPipelineCounterMetric(t, obs.counters, "pipeline_stage_runs_total", 1, "result", "ok")
	assertPipelineHistogramMetric(t, obs.histograms, "pipeline_stage_duration_seconds", "stage", "compression")
	assertPipelineCounterMetric(t, obs.counters, "pipeline_variants_generated_total", 1, "stage", "compression")
	assertPipelineCounterMetric(t, obs.counters, "pipeline_variants_generated_bytes_total", 123, "stage", "compression")
}

func TestPipelineStageMetricsRecordSkippedRuns(t *testing.T) {
	obs := &pipelineRecordingObservability{}
	cat := catalog.NewInMemoryCatalog()
	asset := &catalog.Asset{
		Path:       "hero.png",
		FullPath:   "/tmp/hero.png",
		MediaType:  "image/png",
		SourceHash: "hash-2",
		ETag:       "\"hash-2\"",
	}
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}

	svc := pipeline.NewServiceWithObservabilityForTest(&config.Compression{}, slog.New(slog.DiscardHandler), cat, obs, 1)
	got := pipeline.ExecuteStageTaskForTest(svc, stageResultStage{name: "image", err: pipeline.ErrVariantSkipped}, asset, pipeline.Task{AssetPath: "hero.png", Width: 640})
	if got != nil {
		t.Fatalf("expected nil variant for skipped stage, got %#v", got)
	}

	assertPipelineCounterMetric(t, obs.counters, "pipeline_stage_runs_total", 1, "stage", "image")
	assertPipelineCounterMetric(t, obs.counters, "pipeline_stage_runs_total", 1, "result", "skipped")
}

func pipelineAttrsToMap(attrs []observabilityx.Attribute) map[string]any {
	values := make(map[string]any, len(attrs))
	for _, attr := range attrs {
		values[attr.Key] = attr.Value
	}
	return values
}

func assertPipelineCounterMetric(t *testing.T, metrics []pipelineMetric, name string, wantValue float64, key string, want any) {
	t.Helper()

	for _, metric := range metrics {
		if metric.name != name || metric.value != wantValue {
			continue
		}
		if got := metric.attrs[key]; got == want {
			return
		}
	}
	t.Fatalf("expected counter %s with %s=%v", name, key, want)
}

func assertPipelineHistogramMetric(t *testing.T, metrics []pipelineMetric, name, key string, want any) {
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
