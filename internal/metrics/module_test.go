package metrics_test

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPrometheusAdapterReusesAlreadyRegisteredCollectors(t *testing.T) {
	first := metrics.NewAdapter(slog.New(slog.DiscardHandler))
	second := metrics.NewAdapter(slog.New(slog.DiscardHandler))
	spec := observabilityx.NewCounterSpec(
		"prometheus_adapter_reuse_test_total",
		observabilityx.WithDescription("Prometheus adapter duplicate registration compatibility test."),
		observabilityx.WithLabelKeys("workload", "mode", "result"),
	)
	attrs := []observabilityx.Attribute{
		observabilityx.String("workload", "cache_warm"),
		observabilityx.String("mode", "parallel"),
		observabilityx.String("result", "ok"),
	}

	first.Counter(spec).Add(t.Context(), 1, attrs...)
	second.Counter(spec).Add(t.Context(), 1, attrs...)

	expected := strings.NewReader(`
# HELP spack_prometheus_adapter_reuse_test_total Prometheus adapter duplicate registration compatibility test.
# TYPE spack_prometheus_adapter_reuse_test_total counter
spack_prometheus_adapter_reuse_test_total{mode="parallel",result="ok",workload="cache_warm"} 2
`)
	if err := testutil.GatherAndCompare(prometheus.DefaultGatherer, expected, "spack_prometheus_adapter_reuse_test_total"); err != nil {
		t.Fatal(err)
	}
}
