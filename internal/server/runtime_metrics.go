package server

import (
	"slices"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type RuntimeMetrics struct {
	RequestsInFlight                      prometheus.Gauge
	PreparedSnapshotDuration              prometheus.Gauge
	PreparedSnapshotRoutesCurrent         prometheus.Gauge
	PreparedSnapshotResponsesCurrent      prometheus.Gauge
	PreparedSnapshotBodyEntriesCurrent    prometheus.Gauge
	PreparedSnapshotBodyBytesCurrent      prometheus.Gauge
	PreparedSnapshotRebuilds              *prometheus.CounterVec
	PreparedSnapshotRebuildDuration       *prometheus.HistogramVec
	PreparedSnapshotRebuildWorkerRunning  prometheus.Gauge
	PreparedSnapshotRebuildCoalescedTotal prometheus.Counter
	Readiness                             *prometheus.GaugeVec
	StartupPhase                          *prometheus.GaugeVec
	StartupDuration                       prometheus.Gauge
}

func NewRuntimeMetrics() *RuntimeMetrics {
	metrics := &RuntimeMetrics{
		RequestsInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "spack_http_requests_in_flight",
			Help: "Current number of HTTP requests being processed by the server",
		}),
		PreparedSnapshotDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "spack_prepared_snapshot_duration_seconds",
			Help: "Duration of the latest successful prepared snapshot rebuild in seconds",
		}),
		PreparedSnapshotRoutesCurrent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "spack_prepared_snapshot_routes_current",
			Help: "Current number of routes in the prepared response snapshot",
		}),
		PreparedSnapshotResponsesCurrent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "spack_prepared_snapshot_responses_current",
			Help: "Current number of prepared identity and variant responses",
		}),
		PreparedSnapshotBodyEntriesCurrent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "spack_prepared_snapshot_body_entries_current",
			Help: "Current number of prepared responses with an in-memory body",
		}),
		PreparedSnapshotBodyBytesCurrent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "spack_prepared_snapshot_body_bytes_current",
			Help: "Current bytes held by prepared in-memory response bodies",
		}),
		PreparedSnapshotRebuilds: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "spack_prepared_snapshot_rebuilds_total",
			Help: "Total number of prepared snapshot rebuilds by result",
		}, []string{"result"}),
		PreparedSnapshotRebuildDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "spack_prepared_snapshot_rebuild_duration_seconds",
			Help:    "Prepared snapshot rebuild duration in seconds by result",
			Buckets: prometheus.DefBuckets,
		}, []string{"result"}),
		PreparedSnapshotRebuildWorkerRunning: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "spack_prepared_snapshot_rebuild_worker_running",
			Help: "Whether the asynchronous prepared snapshot rebuild worker is currently running",
		}),
		PreparedSnapshotRebuildCoalescedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "spack_prepared_snapshot_rebuild_coalesced_total",
			Help: "Total number of prepared snapshot rebuild requests coalesced into an active worker",
		}),
		Readiness: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "spack_server_readiness_current",
			Help: "Current server readiness state",
		}, []string{"state"}),
		StartupPhase: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "spack_server_startup_phase_current",
			Help: "Current runtime startup phase",
		}, []string{"phase"}),
		StartupDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "spack_server_startup_duration_seconds",
			Help: "Duration from runtime startup bootstrap start to readiness in seconds",
		}),
	}
	metrics.SetReadiness(false)
	metrics.SetStartupPhase("starting")
	return metrics
}

func (m *RuntimeMetrics) Collectors() []prometheus.Collector {
	if m == nil {
		return nil
	}
	return []prometheus.Collector{
		m.RequestsInFlight,
		m.PreparedSnapshotDuration,
		m.PreparedSnapshotRoutesCurrent,
		m.PreparedSnapshotResponsesCurrent,
		m.PreparedSnapshotBodyEntriesCurrent,
		m.PreparedSnapshotBodyBytesCurrent,
		m.PreparedSnapshotRebuilds,
		m.PreparedSnapshotRebuildDuration,
		m.PreparedSnapshotRebuildWorkerRunning,
		m.PreparedSnapshotRebuildCoalescedTotal,
		m.Readiness,
		m.StartupPhase,
		m.StartupDuration,
	}
}

func (m *RuntimeMetrics) IncRequestsInFlight() {
	if m == nil || m.RequestsInFlight == nil {
		return
	}
	m.RequestsInFlight.Inc()
}

func (m *RuntimeMetrics) DecRequestsInFlight() {
	if m == nil || m.RequestsInFlight == nil {
		return
	}
	m.RequestsInFlight.Dec()
}

func (m *RuntimeMetrics) RecordPreparedSnapshot(duration time.Duration, routes, responses, bodyEntries int, bodyBytes int64) {
	if m == nil {
		return
	}
	if duration < 0 {
		duration = 0
	}
	if routes < 0 {
		routes = 0
	}
	if responses < 0 {
		responses = 0
	}
	if bodyEntries < 0 {
		bodyEntries = 0
	}
	if bodyBytes < 0 {
		bodyBytes = 0
	}
	m.PreparedSnapshotDuration.Set(duration.Seconds())
	m.PreparedSnapshotRoutesCurrent.Set(float64(routes))
	m.PreparedSnapshotResponsesCurrent.Set(float64(responses))
	m.PreparedSnapshotBodyEntriesCurrent.Set(float64(bodyEntries))
	m.PreparedSnapshotBodyBytesCurrent.Set(float64(bodyBytes))
}

func (m *RuntimeMetrics) RecordPreparedSnapshotRebuild(duration time.Duration, result string) {
	if m == nil {
		return
	}
	if duration < 0 {
		duration = 0
	}
	normalizedResult := normalizePreparedSnapshotRebuildResult(result)
	m.PreparedSnapshotRebuilds.WithLabelValues(normalizedResult).Inc()
	m.PreparedSnapshotRebuildDuration.WithLabelValues(normalizedResult).Observe(duration.Seconds())
}

func (m *RuntimeMetrics) SetPreparedSnapshotRebuildWorkerRunning(running bool) {
	if m == nil || m.PreparedSnapshotRebuildWorkerRunning == nil {
		return
	}
	if running {
		m.PreparedSnapshotRebuildWorkerRunning.Set(1)
		return
	}
	m.PreparedSnapshotRebuildWorkerRunning.Set(0)
}

func (m *RuntimeMetrics) IncPreparedSnapshotRebuildCoalesced() {
	if m == nil || m.PreparedSnapshotRebuildCoalescedTotal == nil {
		return
	}
	m.PreparedSnapshotRebuildCoalescedTotal.Inc()
}

func normalizePreparedSnapshotRebuildResult(result string) string {
	if strings.EqualFold(strings.TrimSpace(result), "success") {
		return "success"
	}
	return "error"
}
func (m *RuntimeMetrics) SetReadiness(ready bool) {
	if m == nil || m.Readiness == nil {
		return
	}
	for _, state := range readinessStates {
		m.Readiness.WithLabelValues(state).Set(0)
	}
	if ready {
		m.Readiness.WithLabelValues("ready").Set(1)
		return
	}
	m.Readiness.WithLabelValues("not_ready").Set(1)
}

func (m *RuntimeMetrics) SetStartupPhase(phase string) {
	if m == nil || m.StartupPhase == nil {
		return
	}
	selected := normalizeStartupPhase(phase)
	for _, candidate := range startupPhases {
		m.StartupPhase.WithLabelValues(candidate).Set(0)
	}
	m.StartupPhase.WithLabelValues(selected).Set(1)
}

func (m *RuntimeMetrics) SetStartupDuration(duration time.Duration) {
	if m == nil {
		return
	}
	if duration < 0 {
		duration = 0
	}
	m.StartupDuration.Set(duration.Seconds())
}

var readinessStates = []string{"ready", "not_ready"}

var startupPhases = []string{
	"starting",
	"catalog_scan",
	"cache_warmup",
	"prepared_snapshot",
	"ready",
	"error",
}

func normalizeStartupPhase(phase string) string {
	normalized := strings.TrimSpace(strings.ToLower(phase))
	if slices.Contains(startupPhases, normalized) {
		return normalized
	}
	return "error"
}
