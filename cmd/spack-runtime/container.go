package main

import (
	"errors"
	"fmt"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/spack/internal/appmeta"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/cmdkit"
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
