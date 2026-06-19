package metrics_test

import (
	"strings"
	"testing"
	"time"

	spackconfig "github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRuntimeInfoMetricsExposeConfigAndStartTime(t *testing.T) {
	cfg := spackconfig.DefaultConfig()
	cfg.Compression.Mode = spackconfig.CompressionModeWarmup
	cfg.Compression.Enable = true
	cfg.Debug.Enable = true
	cfg.Frontend.ResourceHints.Enable = true
	cfg.Frontend.ResourceHints.EarlyHints = true
	cfg.Frontend.ImmutableCache.Enable = true
	cfg.Image.Enable = false
	cfg.Logger.Level = "warn"
	cfg.HTTP.MemoryCache.Enable = true
	cfg.HTTP.MemoryCache.Warmup = false
	cfg.Robots.Enable = true

	runtimeMetrics := metrics.NewRuntimeInfoMetrics("spack", &cfg, time.Unix(1_744_190_200, 0).UTC())
	registry := prometheus.NewRegistry()
	for _, collector := range runtimeMetrics.Collectors() {
		registry.MustRegister(collector)
	}

	expected := strings.NewReader(`
# HELP spack_config_info Effective low-cardinality runtime configuration for the current spack instance.
# TYPE spack_config_info gauge
spack_config_info{app="spack",compression_mode="warmup",compression_pipeline_enabled="true",debug_enabled="true",frontend_early_hints_enabled="true",frontend_hints_enabled="true",frontend_immutable_enabled="true",image_enabled="false",logger_level="warn",memory_cache_enabled="true",memory_cache_warmup_enabled="false",robots_enabled="true"} 1
# HELP spack_runtime_start_time_seconds Unix time when the current spack runtime instance was initialized.
# TYPE spack_runtime_start_time_seconds gauge
spack_runtime_start_time_seconds{app="spack"} 1.7441902e+09
`)
	if err := testutil.GatherAndCompare(registry, expected,
		"spack_config_info",
		"spack_runtime_start_time_seconds",
	); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeInfoMetricsFallsBackToDefaultConfig(t *testing.T) {
	runtimeMetrics := metrics.NewRuntimeInfoMetrics("", nil, time.Unix(1, 0).UTC())
	registry := prometheus.NewRegistry()
	for _, collector := range runtimeMetrics.Collectors() {
		registry.MustRegister(collector)
	}

	expected := strings.NewReader(`
# HELP spack_config_info Effective low-cardinality runtime configuration for the current spack instance.
# TYPE spack_config_info gauge
spack_config_info{app="spack",compression_mode="lazy",compression_pipeline_enabled="true",debug_enabled="true",frontend_early_hints_enabled="false",frontend_hints_enabled="true",frontend_immutable_enabled="true",image_enabled="true",logger_level="debug",memory_cache_enabled="true",memory_cache_warmup_enabled="true",robots_enabled="true"} 1
# HELP spack_runtime_start_time_seconds Unix time when the current spack runtime instance was initialized.
# TYPE spack_runtime_start_time_seconds gauge
spack_runtime_start_time_seconds{app="spack"} 1
`)
	if err := testutil.GatherAndCompare(registry, expected,
		"spack_config_info",
		"spack_runtime_start_time_seconds",
	); err != nil {
		t.Fatal(err)
	}
}
