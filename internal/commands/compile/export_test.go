package compilecmd

import (
	"context"
	"errors"
	"log/slog"

	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/compiler"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/oops"
)

type bundleWriterFunc func(context.Context, spackbundle.WriteOptions) (spackbundle.WriteSummary, error)

func (fn bundleWriterFunc) Write(ctx context.Context, options spackbundle.WriteOptions) (spackbundle.WriteSummary, error) {
	return fn(ctx, options)
}

func BundleForTest(assetsRoot, output string) (spackbundle.WriteSummary, error) {
	if spackbundle.IsBundlePath(assetsRoot) {
		return spackbundle.WriteSummary{}, oops.In("compile").Wrap(errors.New("compile input must be an asset directory; .spack bundles are runtime sources, not compile inputs"))
	}
	cfg := config.DefaultConfig()
	cfg.Assets.Root = assetsRoot
	scanner, cleanup, err := newBundleTestScanner(&cfg)
	if err != nil {
		return spackbundle.WriteSummary{}, err
	}
	runtime := compiler.Runtime{
		Config:       &cfg,
		Scanner:      scanner,
		Catalog:      catalog.NewCatalog(),
		BundleWriter: bundleWriterFunc(spackbundle.Write),
	}
	summary, compileErr := compiler.NewService().Compile(context.Background(), compiler.Options{
		AssetsRoot: assetsRoot,
		Output:     output,
		Runtime:    runtime,
	})
	cleanupErr := cleanup()
	if compileErr != nil {
		return spackbundle.WriteSummary{}, oops.Wrapf(compileErr, "compile bundle for test")
	}
	if cleanupErr != nil {
		return spackbundle.WriteSummary{}, oops.Wrapf(cleanupErr, "cleanup test source")
	}
	return summary, nil
}

func newBundleTestScanner(cfg *config.Config) (sourcecatalog.Scanner, func() error, error) {
	srcFactory := source.NewSourceFactory(source.NewResolver(), slog.New(slog.DiscardHandler))
	src, err := srcFactory.LocalFS(&cfg.Assets)
	if err != nil {
		return sourcecatalog.Scanner{}, nil, oops.Wrapf(err, "create source for test bundle")
	}
	registry := contentcoding.NewRegistry(contentcoding.Options{
		BrotliQuality: cfg.Compression.BrotliQuality,
		GzipLevel:     cfg.Compression.GzipLevel,
		ZstdLevel:     cfg.Compression.ZstdLevel,
	}, cfg.Compression.NormalizedEncodings())
	return sourcecatalog.NewScannerWithAssets(src, registry, &cfg.Assets), src.Cleanup, nil
}
