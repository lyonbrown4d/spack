package asyncx_test

import (
	"context"
	cxlist "github.com/arcgolabs/collectionx/list"
	cxset "github.com/arcgolabs/collectionx/set"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"slices"
	"strings"
	"testing"
)

func TestNewSettingsUsesNormalizedAsyncWorkers(t *testing.T) {
	settings := asyncx.NewSettingsForTest(&config.Async{Workers: 5})
	if settings.Size != 5 {
		t.Fatalf("expected settings size 5, got %d", settings.Size)
	}
}

func TestRunListFallsBackToSerialWithoutParallelSettings(t *testing.T) {
	values := cxlist.NewList(1, 2, 3)
	visited := cxlist.NewList[int]()

	err := asyncx.RunListForTest[int](t.Context(), nil, nil, "test_serial", values, func(_ context.Context, value int) error {
		visited.Add(value)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(visited.Values(), []int{1, 2, 3}) {
		t.Fatalf("expected serial visit order [1 2 3], got %v", visited.Values())
	}
}

func TestRunListUsesParallelSettings(t *testing.T) {
	values := cxlist.NewList(1, 2, 3, 4)
	visited := cxset.NewConcurrentSet[int]()

	err := asyncx.RunListForTest[int](t.Context(), nil, &asyncx.Settings{Size: 2}, "test_parallel", values, func(_ context.Context, value int) error {
		visited.Add(value)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	got := visited.Values()
	slices.Sort(got)
	if !slices.Equal(got, []int{1, 2, 3, 4}) {
		t.Fatalf("expected all values visited once, got %v", got)
	}
}

func TestRunListRecordsMetrics(t *testing.T) {
	values := cxlist.NewList(1, 2, 3)
	obs := &recordingObservability{}

	err := asyncx.RunListForTest[int](t.Context(), obs, nil, "asset_cache_warm", values, func(_ context.Context, value int) error {
		_ = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	assertRecordedMetricCount(t, obs.counters, "async_batch_items_total", 1)
	assertRecordedMetricCount(t, obs.counters, "async_batch_runs_total", 1)
	assertRecordedMetricCount(t, obs.histograms, "async_batch_duration_seconds", 1)
	assertRecordedMetricCount(t, obs.counters, "async_task_runs_total", 3)
	assertRecordedMetricCount(t, obs.histograms, "async_task_duration_seconds", 3)
	assertRecordedMetricCount(t, obs.counters, "async_task_submissions_total", 0)

	assertRecordedMetric(t, obs.counters, "async_batch_runs_total", map[string]any{
		"workload": "asset_cache_warm",
		"mode":     "serial",
		"result":   "ok",
	})
	assertRecordedMetric(t, obs.counters, "async_batch_items_total", map[string]any{
		"workload": "asset_cache_warm",
		"mode":     "serial",
	})
}

func TestRunListRecordsParallelSubmissionMetrics(t *testing.T) {
	values := cxlist.NewList(1, 2, 3)
	obs := &recordingObservability{}
	err := asyncx.RunListForTest[int](t.Context(), obs, &asyncx.Settings{Size: 2}, "pipeline_warm", values, func(_ context.Context, value int) error {
		_ = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	assertRecordedMetricCount(t, obs.counters, "async_task_submissions_total", 3)
	assertRecordedMetric(t, obs.counters, "async_batch_runs_total", map[string]any{
		"workload": "pipeline_warm",
		"mode":     "parallel",
		"result":   "ok",
	})
	assertRecordedMetric(t, obs.counters, "async_task_submissions_total", map[string]any{
		"workload": "pipeline_warm",
		"result":   "submitted",
	})
}

func TestRuntimeMetricsExposeConfiguredLimit(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := asyncx.NewRuntimeMetricsForTest(&asyncx.Settings{Size: 3})
	for _, collector := range metrics.Collectors() {
		registry.MustRegister(collector)
	}

	expected := strings.NewReader(`
# HELP spack_async_capacity_current Configured concurrency limit for async batch runs
# TYPE spack_async_capacity_current gauge
spack_async_capacity_current 3
`)
	if err := testutil.GatherAndCompare(registry, expected,
		"spack_async_capacity_current",
	); err != nil {
		t.Fatal(err)
	}
}
