package asyncx

import (
	"context"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/config"
)

// NewSettingsForTest exposes settings construction for external tests.
func NewSettingsForTest(cfg *config.Async) *Settings {
	return newSettings(cfg)
}

// NewRuntimeMetricsForTest exposes runtime metrics construction for external tests.
func NewRuntimeMetricsForTest(settings *Settings) *RuntimeMetrics {
	return NewRuntimeMetrics(settings)
}

// RunListForTest exposes RunList with explicit observability metadata for external tests.
func RunListForTest[T any](
	ctx context.Context,
	obs observabilityx.Observability,
	settings *Settings,
	workload string,
	values *cxlist.List[T],
	run func(context.Context, T) error,
) error {
	return RunList(ctx, obs, settings, workload, values, run)
}
