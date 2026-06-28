package main

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/mapper"
	"github.com/lyonbrown4d/spack/internal/appmeta"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/cmdkit"
	configcmd "github.com/lyonbrown4d/spack/internal/commands/config"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/event"
	spacklogger "github.com/lyonbrown4d/spack/internal/logger"
	"github.com/lyonbrown4d/spack/internal/mapx"
	"github.com/lyonbrown4d/spack/internal/metrics"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/runtime"
	"github.com/lyonbrown4d/spack/internal/server"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/lyonbrown4d/spack/internal/task"
	"github.com/lyonbrown4d/spack/internal/validation"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

const dixRecentEventCapacity = 128

func bindRuntimeRoot(command *cobra.Command) {
	var container *dix.App
	command.PreRunE = func(cmd *cobra.Command, args []string) error {
		dixInstance, err := createRuntimeContainer(cmdkit.ConfigLoadOptions(cmd))
		if err != nil {
			return oops.Wrapf(err, "create runtime container")
		}
		container = dixInstance
		return nil
	}
	command.RunE = func(cmd *cobra.Command, args []string) error {
		if container == nil {
			return oops.In("command").Owner("runtime root").Wrap(errors.New("runtime container was not initialized"))
		}
		if err := container.Run(); err != nil {
			return oops.Wrapf(err, "run runtime container")
		}
		return nil
	}
	command.PostRun = func(cmd *cobra.Command, args []string) {
		if container == nil {
			return
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s", container.Meta()); err != nil {
			cmd.PrintErrln(err)
		}
	}
}

func createRuntimeContainer(loadOptions config.LoadOptions) (*dix.App, error) {
	serverModules := server.Modules()
	modules := make([]dix.Module, 0, 17+len(serverModules))
	modules = append(modules,
		appmeta.Module,
		validation.Module,
		mapx.Module,
		config.NewModule(loadOptions),
		spacklogger.Module,
		metrics.Module,
		catalog.Module,
		spackbundle.Module,
		runtime.Module,
		task.Module,
		asyncx.Module,
		event.Module,
		source.Module,
		sourcecatalog.Module,
		contentcoding.Module,
		assetcache.Module,
		resolver.Module,
	)
	modules = append(modules, serverModules...)
	app := dix.New(
		"spack",
		dix.Modules(modules...),
		dix.RunStopTimeout(dix.DefaultRunStopTimeout),
		dix.RecentEvents(dixRecentEventCapacity),
	)
	if err := app.Validate(); err != nil {
		return nil, oops.In("command").Owner("container").Wrap(err)
	}
	return app, nil
}

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
