package inspectcmd

import (
	"context"

	"github.com/lyonbrown4d/spack/internal/config"
)

type ReportForTest = inspectReport

type BundleSummaryForTest = bundleSummary

func AssetsForTest(ctx context.Context, cfg *config.Config) (ReportForTest, error) {
	return inspectAssets(ctx, cfg)
}
