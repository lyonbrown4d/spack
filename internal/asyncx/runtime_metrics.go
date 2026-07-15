package asyncx

import (
	"github.com/prometheus/client_golang/prometheus"
)

type RuntimeMetrics struct {
	collectors []prometheus.Collector
}

func NewRuntimeMetrics(settings *Settings) *RuntimeMetrics {
	if settings == nil {
		settings = &Settings{Size: 1}
	}

	return &RuntimeMetrics{
		collectors: []prometheus.Collector{
			prometheus.NewGaugeFunc(prometheus.GaugeOpts{
				Name: "spack_async_capacity_current",
				Help: "Configured concurrency limit for async batch runs",
			}, func() float64 {
				return float64(settings.Size)
			}),
		},
	}
}

func (m *RuntimeMetrics) Collectors() []prometheus.Collector {
	if m == nil {
		return nil
	}
	return append([]prometheus.Collector(nil), m.collectors...)
}
