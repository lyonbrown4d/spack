package task_test

import (
	"errors"
	"testing"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"github.com/lyonbrown4d/spack/internal/task"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestRuntimeMetricsExposeSchedulerMonitorMetrics(t *testing.T) {
	metrics := task.NewRuntimeMetrics()
	registry := prometheus.NewRegistry()
	for _, collector := range metrics.Collectors() {
		registry.MustRegister(collector)
	}

	sourceRescan := fakeJob{id: uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"), name: "source_rescan"}
	cacheWarmer := fakeJob{id: uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"), name: "cache_warmer"}

	metrics.SchedulerStarted()
	metrics.JobRegistered(sourceRescan)
	metrics.JobRegistered(cacheWarmer)
	metrics.JobStarted(sourceRescan)
	metrics.JobRunning(sourceRescan)
	metrics.JobExecutionTime(sourceRescan, 2*time.Second)
	metrics.JobSchedulingDelay(sourceRescan, time.Unix(100, 0), time.Unix(101, 500_000_000))
	metrics.JobCompleted(sourceRescan)
	metrics.JobStarted(cacheWarmer)
	metrics.ConcurrencyLimitReached("singleton", cacheWarmer)
	metrics.JobExecutionTime(cacheWarmer, 750*time.Millisecond)
	metrics.JobSchedulingDelay(cacheWarmer, time.Unix(200, 0), time.Unix(200, 250_000_000))
	metrics.JobFailed(cacheWarmer, errors.New("boom"))
	metrics.JobUnregistered(sourceRescan)
	metrics.JobUnregistered(cacheWarmer)
	metrics.SchedulerStopped()
	metrics.SchedulerShutdown()

	families := gatherMetricFamilies(t, registry)
	assertSchedulerLifecycleMetrics(t, families)
	assertSchedulerJobEventMetrics(t, families)
	assertSchedulerGaugeMetrics(t, families)
	assertSchedulerHistogramMetrics(t, families)
}

type fakeJob struct {
	id   uuid.UUID
	name string
	tags []string
}

func (j fakeJob) ID() uuid.UUID {
	return j.id
}

func (fakeJob) IsRunning() (bool, error) {
	return false, nil
}

func (fakeJob) LastRun() (time.Time, error) {
	return time.Time{}, nil
}

func (fakeJob) LastRunCompletedAt() (time.Time, error) {
	return time.Time{}, nil
}

func (fakeJob) LastRunStartedAt() (time.Time, error) {
	return time.Time{}, nil
}

func (j fakeJob) Name() string {
	return j.name
}

func (fakeJob) NextRun() (time.Time, error) {
	return time.Time{}, nil
}

func (fakeJob) NextRuns(int) ([]time.Time, error) {
	return nil, nil
}

func (fakeJob) RunNow() error {
	return nil
}

func (fakeJob) Schedule() gocron.JobSchedule {
	return nil
}

func (j fakeJob) Tags() []string {
	return j.tags
}

var _ gocron.Job = fakeJob{}

func labels(values ...string) map[string]string {
	out := make(map[string]string, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		out[values[i]] = values[i+1]
	}
	return out
}

func assertSchedulerLifecycleMetrics(t *testing.T, families []*dto.MetricFamily) {
	t.Helper()
	assertGaugeValue(t, families, "spack_task_scheduler_running", nil)
	assertCounterValue(t, families, "spack_task_scheduler_events_total", labels("event", "started"))
	assertCounterValue(t, families, "spack_task_scheduler_events_total", labels("event", "stopped"))
	assertCounterValue(t, families, "spack_task_scheduler_events_total", labels("event", "shutdown"))
}

func assertSchedulerJobEventMetrics(t *testing.T, families []*dto.MetricFamily) {
	t.Helper()
	assertCounterValue(t, families, "spack_task_scheduler_job_events_total", labels("job", "source_rescan", "event", "registered"))
	assertCounterValue(t, families, "spack_task_scheduler_job_events_total", labels("job", "source_rescan", "event", "started"))
	assertCounterValue(t, families, "spack_task_scheduler_job_events_total", labels("job", "source_rescan", "event", "running"))
	assertCounterValue(t, families, "spack_task_scheduler_job_events_total", labels("job", "source_rescan", "event", "completed"))
	assertCounterValue(t, families, "spack_task_scheduler_job_events_total", labels("job", "source_rescan", "event", "unregistered"))
	assertCounterValue(t, families, "spack_task_scheduler_job_events_total", labels("job", "cache_warmer", "event", "failed"))
	assertCounterValue(t, families, "spack_task_scheduler_concurrency_limit_total", labels("job", "cache_warmer", "limit_type", "singleton"))
}

func assertSchedulerGaugeMetrics(t *testing.T, families []*dto.MetricFamily) {
	t.Helper()
	assertGaugeValue(t, families, "spack_task_scheduler_jobs_registered_current", labels("job", "source_rescan"))
	assertGaugeValue(t, families, "spack_task_scheduler_jobs_registered_current", labels("job", "cache_warmer"))
	assertGaugeValue(t, families, "spack_task_scheduler_jobs_running_current", labels("job", "source_rescan"))
	assertGaugeValue(t, families, "spack_task_scheduler_jobs_running_current", labels("job", "cache_warmer"))
}

func assertSchedulerHistogramMetrics(t *testing.T, families []*dto.MetricFamily) {
	t.Helper()
	assertHistogramSample(t, families, "spack_task_scheduler_job_execution_seconds", labels("job", "source_rescan"), 2)
	assertHistogramSample(t, families, "spack_task_scheduler_job_execution_seconds", labels("job", "cache_warmer"), 0.75)
	assertHistogramSample(t, families, "spack_task_scheduler_job_scheduling_delay_seconds", labels("job", "source_rescan"), 1.5)
	assertHistogramSample(t, families, "spack_task_scheduler_job_scheduling_delay_seconds", labels("job", "cache_warmer"), 0.25)
}

func gatherMetricFamilies(t *testing.T, registry *prometheus.Registry) []*dto.MetricFamily {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	return families
}

func assertCounterValue(t *testing.T, families []*dto.MetricFamily, name string, wantLabels map[string]string) {
	t.Helper()
	metric := findMetric(t, families, name, wantLabels)
	if got := metric.GetCounter().GetValue(); got != 1 {
		t.Fatalf("expected %s labels %v counter value 1, got %v", name, wantLabels, got)
	}
}

func assertGaugeValue(t *testing.T, families []*dto.MetricFamily, name string, wantLabels map[string]string) {
	t.Helper()
	metric := findMetric(t, families, name, wantLabels)
	if got := metric.GetGauge().GetValue(); got != 0 {
		t.Fatalf("expected %s labels %v gauge value 0, got %v", name, wantLabels, got)
	}
}

func assertHistogramSample(t *testing.T, families []*dto.MetricFamily, name string, wantLabels map[string]string, wantSum float64) {
	t.Helper()
	metric := findMetric(t, families, name, wantLabels)
	histogram := metric.GetHistogram()
	if got := histogram.GetSampleCount(); got != 1 {
		t.Fatalf("expected %s labels %v histogram count 1, got %d", name, wantLabels, got)
	}
	if got := histogram.GetSampleSum(); got != wantSum {
		t.Fatalf("expected %s labels %v histogram sum %v, got %v", name, wantLabels, wantSum, got)
	}
}

func findMetric(t *testing.T, families []*dto.MetricFamily, name string, wantLabels map[string]string) *dto.Metric {
	t.Helper()
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metricLabelsMatch(metric, wantLabels) {
				return metric
			}
		}
	}
	t.Fatalf("metric %s with labels %v not found", name, wantLabels)
	return nil
}

func metricLabelsMatch(metric *dto.Metric, wantLabels map[string]string) bool {
	if len(wantLabels) == 0 {
		return len(metric.GetLabel()) == 0
	}
	if len(metric.GetLabel()) != len(wantLabels) {
		return false
	}
	for _, label := range metric.GetLabel() {
		if wantValue, ok := wantLabels[label.GetName()]; !ok || wantValue != label.GetValue() {
			return false
		}
	}
	return true
}
