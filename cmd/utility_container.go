package cmd

import (
	"fmt"
	"log/slog"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/lyonbrown4d/spack/internal/validation"
)

func resolveConfigWithDix(loadOptions config.LoadOptions) (*config.Config, error) {
	rt, err := buildUtilityRuntime(
		"spack-config",
		validation.Module,
		config.NewModule(loadOptions),
	)
	if err != nil {
		return nil, err
	}
	cfg, err := dix.ResolveAs[*config.Config](rt.Container())
	if err != nil {
		return nil, fmt.Errorf("resolve config: %w", err)
	}
	return cfg, nil
}

func resolveScannerWithDix(cfg *config.Config) (sourcecatalog.Scanner, error) {
	rt, err := buildUtilityRuntime(
		"spack-inspect",
		inspectConfigModule(cfg),
		contentcoding.Module,
		source.Module,
		sourcecatalog.Module,
	)
	if err != nil {
		return sourcecatalog.Scanner{}, err
	}
	scanner, err := dix.ResolveAs[sourcecatalog.Scanner](rt.Container())
	if err != nil {
		return sourcecatalog.Scanner{}, fmt.Errorf("resolve source scanner: %w", err)
	}
	return scanner, nil
}

func buildUtilityRuntime(name string, modules ...dix.Module) (*dix.Runtime, error) {
	app := dix.New(name, dix.Modules(modules...))
	if err := app.Validate(); err != nil {
		return nil, fmt.Errorf("validate %s container: %w", name, err)
	}
	rt, err := app.Build()
	if err != nil {
		return nil, fmt.Errorf("build %s container: %w", name, err)
	}
	return rt, nil
}

func inspectConfigModule(cfg *config.Config) dix.Module {
	return dix.NewModule("inspect-config",
		dix.WithModuleProviders(
			dix.Value(cfg),
			dix.Provider1(func(cfg *config.Config) *config.Assets { return &cfg.Assets }),
			dix.Provider1(func(cfg *config.Config) *config.Compression { return &cfg.Compression }),
			dix.Provider0(func() *slog.Logger { return slog.New(slog.DiscardHandler) }),
		),
	)
}
