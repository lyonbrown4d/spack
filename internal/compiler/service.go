// Package compiler implements the frontend asset compile use case.
package compiler

import (
	"context"
	"encoding/csv"
	"errors"
	"strings"

	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
  "github.com/samber/lo"
  "github.com/samber/oops"
	"github.com/spf13/pflag"
)

// Service compiles frontend assets into a SPACK bundle.
type Service struct{}

// NewService returns a compile use case service.
func NewService() Service {
	return Service{}
}

// Options configures a compile run.
type Options struct {
	AssetsRoot  string
	Output      string
	LoadOptions config.LoadOptions
}

// Compile scans assets, generates variants, and writes a SPACK bundle.
func (Service) Compile(ctx context.Context, options Options) (spackbundle.WriteSummary, error) {
	if err := validateCompileInput(options.AssetsRoot); err != nil {
		return spackbundle.WriteSummary{}, err
	}
	loadOptions, err := compileLoadOptions(options.AssetsRoot, options.LoadOptions)
	if err != nil {
		return spackbundle.WriteSummary{}, err
	}
	cfg, err := cmdkit.ResolveConfigWithDix(loadOptions)
	if err != nil {
		return spackbundle.WriteSummary{}, oops.Wrapf(err, "resolve compile config")
	}
	runtime, err := cmdkit.ResolveCompilerWithDix(cfg)
	if err != nil {
		return spackbundle.WriteSummary{}, oops.Wrapf(err, "resolve compiler runtime")
	}
	snapshot, err := runtime.Scanner.Scan(ctx)
	if err != nil {
		return spackbundle.WriteSummary{}, oops.Wrapf(err, "scan assets")
	}
	if upsertErr := upsertCompileSnapshot(runtime.Catalog, snapshot); upsertErr != nil {
		return spackbundle.WriteSummary{}, upsertErr
	}
	if warmErr := runtime.Pipeline.Warm(ctx); warmErr != nil {
		return spackbundle.WriteSummary{}, oops.Wrapf(warmErr, "generate bundle variants")
	}
	summary, err := runtime.BundleWriter.Write(ctx, spackbundle.WriteOptions{
		Output: options.Output,
		Root:   cfg.Assets.Root,
		Files:  bundleFilesFromCatalog(cfg.Assets.Root, options.Output, runtime.Catalog),
	})
	if err != nil {
		return spackbundle.WriteSummary{}, oops.Wrapf(err, "write spack bundle")
	}
	return summary, nil
}

func validateCompileInput(root string) error {
	if spackbundle.IsBundlePath(root) {
		return oops.In("compile").Wrap(errors.New("compile input must be an asset directory; .spack bundles are runtime sources, not compile inputs"))
	}
	return nil
}

func compileLoadOptions(assetsRoot string, base config.LoadOptions) (config.LoadOptions, error) {
	flags, err := cloneVisitedConfigFlags(base.FlagSet)
	if err != nil {
		return config.LoadOptions{}, err
	}
	if err := flags.Set("assets.root", assetsRoot); err != nil {
		return config.LoadOptions{}, oops.Wrapf(err, "set compile assets root")
	}
	return config.LoadOptions{
		Files:   lo.Clone(base.Files),
		FlagSet: flags,
	}, nil
}

func cloneVisitedConfigFlags(source *pflag.FlagSet) (*pflag.FlagSet, error) {
	flags := cmdkit.NewConfigFlagSet()
	if source == nil {
		return flags, nil
	}
	var cloneErr error
	source.Visit(func(flag *pflag.Flag) {
		if flags.Lookup(flag.Name) == nil {
			return
		}
		value, err := cloneConfigFlagValue(flag)
		if err != nil {
			cloneErr = oops.Wrapf(err, "clone config flag %s", flag.Name)
			return
		}
		if err := flags.Set(flag.Name, value); err != nil {
			cloneErr = oops.Wrapf(err, "clone config flag %s", flag.Name)
		}
	})
	if cloneErr != nil {
		return nil, cloneErr
	}
	return flags, nil
}

func cloneConfigFlagValue(flag *pflag.Flag) (string, error) {
	if slice, ok := flag.Value.(pflag.SliceValue); ok {
		return encodeStringSliceFlagValue(slice.GetSlice())
	}
	return flag.Value.String(), nil
}

func encodeStringSliceFlagValue(values []string) (string, error) {
	var builder strings.Builder
	writer := csv.NewWriter(&builder)
	if err := writer.Write(values); err != nil {
		return "", oops.Wrapf(err, "write string slice flag value")
	}
	writer.Flush()
	return strings.TrimSuffix(builder.String(), "\n"), nil
}
