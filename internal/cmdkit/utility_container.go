package cmdkit

import (
	"log/slog"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/mapper"
	"github.com/lyonbrown4d/spack/internal/artifact"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/catalog"
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
	"github.com/samber/oops"
)

type ConfigRuntime struct {
	Config *config.Config
	Mapper *mapper.Mapper
}

func ResolveConfigWithDix(loadOptions config.LoadOptions) (*config.Config, error) {
	rt, err := ResolveConfigRuntimeWithDix(loadOptions)
	if err != nil {
		return nil, err
	}
	return rt.Config, nil
}

func ResolveConfigRuntimeWithDix(loadOptions config.LoadOptions) (ConfigRuntime, error) {
	rt, err := buildUtilityRuntime(
		"spack-config",
		validation.Module,
		config.NewModule(loadOptions),
	)
	if err != nil {
		return ConfigRuntime{}, err
	}
	cfg, err := dix.ResolveAs[*config.Config](rt.Container())
	if err != nil {
		return ConfigRuntime{}, oops.Wrapf(err, "resolve config")
	}
	instance, err := dix.ResolveAs[*mapper.Mapper](rt.Container())
	if err != nil {
		return ConfigRuntime{}, oops.Wrapf(err, "resolve mapper")
	}
	return ConfigRuntime{
		Config: cfg,
		Mapper: instance,
	}, nil
}

func ResolveScannerWithDix(cfg *config.Config) (sourcecatalog.Scanner, error) {
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

type CompilerRuntime struct {
	Scanner      sourcecatalog.Scanner
	Catalog      catalog.Catalog
	Pipeline     *pipeline.Service
	BundleWriter spackbundle.BundleWriter
}

func ResolveCompilerWithDix(cfg *config.Config) (CompilerRuntime, error) {
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
		return CompilerRuntime{}, err
	}
	scanner, err := dix.ResolveAs[sourcecatalog.Scanner](rt.Container())
	if err != nil {
		return CompilerRuntime{}, oops.Wrapf(err, "resolve source scanner")
	}
	cat, err := dix.ResolveAs[catalog.Catalog](rt.Container())
	if err != nil {
		return CompilerRuntime{}, oops.Wrapf(err, "resolve catalog")
	}
	pipelineSvc, err := dix.ResolveAs[*pipeline.Service](rt.Container())
	if err != nil {
		return CompilerRuntime{}, oops.Wrapf(err, "resolve compiler pipeline")
	}
	bundleWriter, err := dix.ResolveAs[spackbundle.BundleWriter](rt.Container())
	if err != nil {
		return CompilerRuntime{}, oops.Wrapf(err, "resolve bundle writer")
	}
	return CompilerRuntime{
		Scanner:      scanner,
		Catalog:      cat,
		Pipeline:     pipelineSvc,
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

func buildUtilityRuntime(name string, modules ...dix.Module) (*dix.Runtime, error) {
	allModules := make([]dix.Module, 0, len(modules)+1)
	allModules = append(allModules, mapx.Module)
	allModules = append(allModules, modules...)
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
