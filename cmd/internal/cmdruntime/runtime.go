// Package cmdruntime provides shared command-side DI runtime helpers.
package cmdruntime

import (
	"log/slog"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/mapper"
	configcmd "github.com/lyonbrown4d/spack/internal/commands/config"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/mapx"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/lyonbrown4d/spack/internal/validation"
	"github.com/samber/oops"
)

func ResolveConfigWithDix(loadOptions config.LoadOptions) (*config.Config, error) {
	rt, err := ResolveConfigRuntimeWithDix(loadOptions)
	if err != nil {
		return nil, err
	}
	return rt.Config, nil
}

func ResolveConfigRuntimeWithDix(loadOptions config.LoadOptions) (configcmd.Runtime, error) {
	rt, err := BuildUtilityRuntime(
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

func ResolveScannerWithDix(cfg *config.Config) (sourcecatalog.Scanner, error) {
	rt, err := BuildUtilityRuntime(
		"spack-inspect",
		InspectConfigModule(cfg),
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

func BuildUtilityRuntime(name string, modules ...dix.Module) (*dix.Runtime, error) {
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

func InspectConfigModule(cfg *config.Config) dix.Module {
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
