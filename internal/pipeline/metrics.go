package pipeline

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	QueueLength              prometheus.Gauge
	QueueCapacity            prometheus.Gauge
	EnqueueDroppedTotal      prometheus.Counter
	EnqueueDeduplicatedTotal prometheus.Counter
	CleanupRunsTotal         prometheus.Counter
	CleanupRemovedTotal      *prometheus.CounterVec
	CleanupRemovedBytesTotal *prometheus.CounterVec
}

func newMetrics() *Metrics {
	return &Metrics{
		QueueLength: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "spack_pipeline_queue_length",
			Help: "Current number of pending legacy lazy pipeline requests in queue",
		}),
		QueueCapacity: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "spack_pipeline_queue_capacity",
			Help: "Configured legacy lazy pipeline queue capacity",
		}),
		EnqueueDroppedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "spack_pipeline_enqueue_dropped_total",
			Help: "Total number of legacy lazy pipeline enqueue requests dropped due to a full queue",
		}),
		EnqueueDeduplicatedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "spack_pipeline_enqueue_deduplicated_total",
			Help: "Total number of deduplicated legacy lazy pipeline enqueue requests",
		}),
		CleanupRunsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "spack_pipeline_cleanup_runs_total",
			Help: "Total number of pipeline cache cleanup runs",
		}),
		CleanupRemovedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "spack_pipeline_cleanup_removed_total",
			Help: "Total number of removed pipeline cache files",
		}, []string{"reason"}),
		CleanupRemovedBytesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "spack_pipeline_cleanup_removed_bytes_total",
			Help: "Total size in bytes removed by pipeline cache cleanup",
		}, []string{"reason"}),
	}
}

func (m *Metrics) Collectors() []prometheus.Collector {
	if m == nil {
		return nil
	}
	return []prometheus.Collector{
		m.QueueLength,
		m.QueueCapacity,
		m.EnqueueDroppedTotal,
		m.EnqueueDeduplicatedTotal,
		m.CleanupRunsTotal,
		m.CleanupRemovedTotal,
		m.CleanupRemovedBytesTotal,
	}
}
