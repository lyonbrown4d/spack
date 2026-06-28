package inspectcmd

import (
	"context"
	"log/slog"

	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/samber/oops"
)

type ReportForTest = inspectReport

type BundleSummaryForTest = bundleSummary

func AssetsForTest(ctx context.Context, cfg *config.Config) (ReportForTest, error) {
	return inspectAssets(ctx, cfg, scannerForTest)
}

func scannerForTest(cfg *config.Config) (sourcecatalog.Scanner, error) {
	srcFactory := source.NewSourceFactory(source.NewResolver(), slog.New(slog.DiscardHandler))
	src, err := srcFactory.LocalFS(&cfg.Assets)
	if err != nil {
		return sourcecatalog.Scanner{}, oops.Wrapf(err, "create source scanner for test")
	}
	registry := contentcoding.NewRegistry(contentcoding.Options{
		BrotliQuality: cfg.Compression.BrotliQuality,
		GzipLevel:     cfg.Compression.GzipLevel,
		ZstdLevel:     cfg.Compression.ZstdLevel,
	}, cfg.Compression.NormalizedEncodings())
	return sourcecatalog.NewScannerWithAssets(src, registry, &cfg.Assets), nil
}
