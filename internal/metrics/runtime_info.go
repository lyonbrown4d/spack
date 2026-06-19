package metrics

import (
	"strconv"
	"time"

	spackconfig "github.com/lyonbrown4d/spack/internal/config"
	"github.com/prometheus/client_golang/prometheus"
)

type RuntimeInfoMetrics struct {
	collectors []prometheus.Collector
}

func NewRuntimeInfoMetrics(appName string, cfg *spackconfig.Config, startedAt time.Time) *RuntimeInfoMetrics {
	if cfg == nil {
		cfg = new(spackconfig.DefaultConfig())
	}
	if startedAt.IsZero() {
		startedAt = time.Now()
	}

	configInfo := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "spack_config_info",
		Help: "Effective low-cardinality runtime configuration for the current spack instance.",
		ConstLabels: prometheus.Labels{
			"app":                          normalizeBuildInfoLabel(appName, "spack"),
			"compression_mode":             cfg.Compression.NormalizedMode(),
			"compression_pipeline_enabled": strconv.FormatBool(cfg.Compression.PipelineEnabled()),
			"debug_enabled":                strconv.FormatBool(cfg.Debug.Enable),
			"frontend_early_hints_enabled": strconv.FormatBool(cfg.Frontend.ResourceHints.EarlyHints),
			"frontend_immutable_enabled":   strconv.FormatBool(cfg.Frontend.ImmutableCache.Enable),
			"frontend_hints_enabled":       strconv.FormatBool(cfg.Frontend.ResourceHints.Enabled()),
			"image_enabled":                strconv.FormatBool(cfg.Image.Enable),
			"logger_level":                 cfg.Logger.Level,
			"memory_cache_enabled":         strconv.FormatBool(cfg.HTTP.MemoryCache.Enabled()),
			"memory_cache_warmup_enabled":  strconv.FormatBool(cfg.HTTP.MemoryCache.WarmupEnabled()),
			"metrics_enabled":              strconv.FormatBool(cfg.Metrics.Enable),
			"robots_enabled":               strconv.FormatBool(cfg.Robots.Enable),
		},
	}, func() float64 {
		return 1
	})

	startTime := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "spack_runtime_start_time_seconds",
		Help: "Unix time when the current spack runtime instance was initialized.",
		ConstLabels: prometheus.Labels{
			"app": normalizeBuildInfoLabel(appName, "spack"),
		},
	}, func() float64 {
		return float64(startedAt.Unix())
	})

	return &RuntimeInfoMetrics{
		collectors: []prometheus.Collector{configInfo, startTime},
	}
}

func (m *RuntimeInfoMetrics) Collectors() []prometheus.Collector {
	if m == nil {
		return nil
	}
	return m.collectors
}
