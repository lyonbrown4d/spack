// Package compiler implements the frontend asset compile use case.
package compiler

import (
	"context"
	"errors"

	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/oops"
)

// VariantGenerator generates compiler-managed asset variants before bundle writing.
type VariantGenerator interface {
	Warm(context.Context) error
}

// Runtime contains the business capabilities required by one compile run.
type Runtime struct {
	Config       *config.Config
	Scanner      sourcecatalog.Scanner
	Catalog      catalog.Catalog
	Generator    VariantGenerator
	BundleWriter spackbundle.BundleWriter
}

// Service compiles frontend assets into a SPACK bundle.
type Service struct{}

// NewService returns a compile use case service.
func NewService() Service {
	return Service{}
}

// Options configures a compile run.
type Options struct {
	AssetsRoot string
	Output     string
	Runtime    Runtime
}

// Compile scans assets, generates variants, and writes a SPACK bundle.
func (Service) Compile(ctx context.Context, options Options) (spackbundle.WriteSummary, error) {
	if err := validateCompileInput(options.AssetsRoot); err != nil {
		return spackbundle.WriteSummary{}, err
	}
	if err := validateCompileRuntime(options.Runtime); err != nil {
		return spackbundle.WriteSummary{}, err
	}

	runtime := options.Runtime
	snapshot, err := runtime.Scanner.Scan(ctx)
	if err != nil {
		return spackbundle.WriteSummary{}, oops.Wrapf(err, "scan assets")
	}
	if upsertErr := upsertCompileSnapshot(runtime.Catalog, snapshot); upsertErr != nil {
		return spackbundle.WriteSummary{}, upsertErr
	}
	if runtime.Generator != nil {
		if warmErr := runtime.Generator.Warm(ctx); warmErr != nil {
			return spackbundle.WriteSummary{}, oops.Wrapf(warmErr, "generate bundle variants")
		}
	}
	summary, err := runtime.BundleWriter.Write(ctx, spackbundle.WriteOptions{
		Output: options.Output,
		Root:   runtime.Config.Assets.Root,
		Files:  bundleFilesFromCatalog(runtime.Config.Assets.Root, options.Output, runtime.Catalog),
	})
	if err != nil {
		return spackbundle.WriteSummary{}, oops.Wrapf(err, "write spack bundle")
	}
	return summary, nil
}

func validateCompileRuntime(runtime Runtime) error {
	if runtime.Config == nil {
		return oops.In("compile").Owner("runtime").Wrap(errors.New("config is required"))
	}
	if runtime.Catalog == nil {
		return oops.In("compile").Owner("runtime").Wrap(errors.New("catalog is required"))
	}
	if runtime.BundleWriter == nil {
		return oops.In("compile").Owner("runtime").Wrap(errors.New("bundle writer is required"))
	}
	return nil
}

func validateCompileInput(root string) error {
	if spackbundle.IsBundlePath(root) {
		return oops.In("compile").Wrap(errors.New("compile input must be an asset directory; .spack bundles are runtime sources, not compile inputs"))
	}
	return nil
}
