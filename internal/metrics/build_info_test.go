package metrics_test

import (
	"runtime/debug"
	"strings"
	"testing"

	"github.com/lyonbrown4d/spack/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestBuildInfoMetricsExposeStaticLabels(t *testing.T) {
	buildInfoMetrics := metrics.NewBuildInfoMetricsForTest("spack", &debug.BuildInfo{
		GoVersion: "go1.25.0",
		Path:      "github.com/lyonbrown4d/spack",
		Main: debug.Module{
			Version: "v1.2.3",
		},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abc123"},
			{Key: "vcs.time", Value: "2026-04-09T10:00:00Z"},
			{Key: "vcs.modified", Value: "true"},
		},
	})

	registry := prometheus.NewRegistry()
	for _, collector := range buildInfoMetrics.Collectors() {
		registry.MustRegister(collector)
	}

	expected := strings.NewReader(`
# HELP spack_build_info Build metadata for the current spack runtime.
# TYPE spack_build_info gauge
spack_build_info{app="spack",go_version="go1.25.0",module_path="github.com/lyonbrown4d/spack",vcs_modified="true",vcs_revision="abc123",vcs_time="2026-04-09T10:00:00Z",version="v1.2.3"} 1
`)
	if err := testutil.GatherAndCompare(registry, expected, "spack_build_info"); err != nil {
		t.Fatal(err)
	}
}

func TestBuildInfoMetricsFallbackLabels(t *testing.T) {
	buildInfoMetrics := metrics.NewBuildInfoMetricsForTest("  ", &debug.BuildInfo{
		Main: debug.Module{
			Version: "(devel)",
		},
	})

	registry := prometheus.NewRegistry()
	for _, collector := range buildInfoMetrics.Collectors() {
		registry.MustRegister(collector)
	}

	expected := strings.NewReader(`
# HELP spack_build_info Build metadata for the current spack runtime.
# TYPE spack_build_info gauge
spack_build_info{app="spack",go_version="unknown",module_path="unknown",vcs_modified="unknown",vcs_revision="unknown",vcs_time="unknown",version="unknown"} 1
`)
	if err := testutil.GatherAndCompare(registry, expected, "spack_build_info"); err != nil {
		t.Fatal(err)
	}
}
