package main

import (
	"encoding/csv"
	"log/slog"
	"strings"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/mapper"
	"github.com/lyonbrown4d/spack/internal/artifact"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/cmdkit"
	configcmd "github.com/lyonbrown4d/spack/internal/commands/config"
	"github.com/lyonbrown4d/spack/internal/compiler"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/event"
	"github.com/lyonbrown4d/spack/internal/mapx"
	"github.com/lyonbrown4d/spack/internal/metrics"
	"github.com/lyonbrown4d/spack/internal/pipeline"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/lyonbrown4d/spack/internal/validation"
	"github.com/samber/lo"
	"github.com/samber/oops"
	"github.com/spf13/pflag"
)

func resolveConfigWithDix(loadOptions config.LoadOptions) (*config.Config, error) {
	rt, err := resolveConfigRuntimeWithDix(loadOptions)
	if err != nil {
		return nil, err
	}
	return rt.Config, nil
}

func resolveConfigRuntimeWithDix(loadOptions config.LoadOptions) (configcmd.Runtime, error) {
	rt, err := buildUtilityRuntime(
		"spack-config",
		validation.Module,
		config.NewModule(loadOptions),
	)
	if err != nil {
		return configcmd.Runtime{}, err
	}
	cfg, err := dix.ResolveAs[*config.Config](rt.Container())
	if err != nil {
		return configcmd.Runtime{}, oops.Wrapf(err, "resolve config")
	}
	instance, err := dix.ResolveAs[*mapper.Mapper](rt.Container())
	if err != nil {
		return configcmd.Runtime{}, oops.Wrapf(err, "resolve mapper")
	}
	return configcmd.Runtime{Config: cfg, Mapper: instance}, nil
}

func resolveScannerWithDix(cfg *config.Config) (sourcecatalog.Scanner, error) {
	rt, err := buildUtilityRuntime(
		"spack-inspect",
		inspectConfigModule(cfg),
		contentcoding.Module,
		source.Module,
		sourcecatalog.Module,
		spackbundle.Module,
	)
	if err != nil {
		return sourcecatalog.Scanner{}, err
	}
	scanner, err := dix.ResolveAs[sourcecatalog.Scanner](rt.Container())
	if err != nil {
		return sourcecatalog.Scanner{}, oops.Wrapf(err, "resolve source scanner")
	}
	return scanner, nil
}

func resolveCompilerRuntimeWithDix(loadOptions config.LoadOptions, assetsRoot string) (compiler.Runtime, error) {
	compileOptions, err := compileLoadOptions(assetsRoot, loadOptions)
	if err != nil {
		return compiler.Runtime{}, err
	}
	cfg, err := resolveConfigWithDix(compileOptions)
	if err != nil {
		return compiler.Runtime{}, oops.Wrapf(err, "resolve compile config")
	}
	compilerCfg := compilerConfigForGeneration(cfg)
	rt, err := buildUtilityRuntime(
		"spack-compiler",
		inspectConfigModule(compilerCfg),
		metrics.Module,
		catalog.Module,
		asyncx.Module,
		event.Module,
		contentcoding.Module,
		source.Module,
		sourcecatalog.Module,
		spackbundle.Module,
		artifact.Module,
		pipeline.Module,
	)
	if err != nil {
		return compiler.Runtime{}, err
	}
	scanner, err := dix.ResolveAs[sourcecatalog.Scanner](rt.Container())
	if err != nil {
		return compiler.Runtime{}, oops.Wrapf(err, "resolve source scanner")
	}
	cat, err := dix.ResolveAs[catalog.Catalog](rt.Container())
	if err != nil {
		return compiler.Runtime{}, oops.Wrapf(err, "resolve catalog")
	}
	pipelineSvc, err := dix.ResolveAs[*pipeline.Service](rt.Container())
	if err != nil {
		return compiler.Runtime{}, oops.Wrapf(err, "resolve compiler pipeline")
	}
	bundleWriter, err := dix.ResolveAs[spackbundle.BundleWriter](rt.Container())
	if err != nil {
		return compiler.Runtime{}, oops.Wrapf(err, "resolve bundle writer")
	}
	return compiler.Runtime{
		Config:       compilerCfg,
		Scanner:      scanner,
		Catalog:      cat,
		Generator:    pipelineSvc,
		BundleWriter: bundleWriter,
	}, nil
}

func compilerConfigForGeneration(cfg *config.Config) *config.Config {
	if cfg == nil {
		return nil
	}
	compilerCfg := *cfg
	if compilerCfg.Compression.Enable && compilerCfg.Compression.NormalizedMode() != config.CompressionModeOff {
		compilerCfg.Compression.Mode = config.CompressionModeWarmup
	}
	return &compilerCfg
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

func cloneVisitedConfigFlags(sourceFlags *pflag.FlagSet) (*pflag.FlagSet, error) {
	flags := cmdkit.NewConfigFlagSet()
	if sourceFlags == nil {
		return flags, nil
	}
	var cloneErr error
	sourceFlags.Visit(func(flag *pflag.Flag) {
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

func buildUtilityRuntime(name string, modules ...dix.Module) (*dix.Runtime, error) {
	allModules := append([]dix.Module{mapx.Module}, modules...)
	app := dix.New(name, dix.Modules(allModules...))
	if err := app.Validate(); err != nil {
		return nil, oops.Wrapf(err, "validate %s container", name)
	}
	rt, err := app.Build()
	if err != nil {
		return nil, oops.Wrapf(err, "build %s container", name)
	}
	return rt, nil
}

func inspectConfigModule(cfg *config.Config) dix.Module {
	return dix.NewModule("inspect-config",
		dix.WithModuleProviders(
			dix.Value(cfg),
			dix.Provider1(func(cfg *config.Config) *config.Assets { return &cfg.Assets }),
			dix.Provider1(func(cfg *config.Config) *config.Async { return &cfg.Async }),
			dix.Provider1(func(cfg *config.Config) *config.Compression { return &cfg.Compression }),
			dix.Provider1(func(cfg *config.Config) *config.Image { return &cfg.Image }),
			dix.Provider0(func() *slog.Logger { return slog.New(slog.DiscardHandler) }),
		),
	)
}
