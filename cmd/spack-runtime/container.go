package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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
		if err := runRuntimeContainer(container); err != nil {
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

func runRuntimeContainer(app *dix.App) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runRuntimeContainerContext(ctx, app)
}

func runRuntimeContainerContext(ctx context.Context, app *dix.App) error {
	dixRuntime, err := app.Start(ctx)
	if err != nil {
		return oops.In("command").Owner("runtime runner").Wrap(err)
	}

	fatalSignal, resolveErr := dix.ResolveAs[*runtime.FatalSignal](dixRuntime.Container())
	if resolveErr != nil {
		stopErr := stopRuntimeContainer(ctx, app, dixRuntime)
		return oops.In("command").Owner("runtime runner").Wrap(errors.Join(resolveErr, stopErr))
	}

	runErr := waitForRuntimeExit(ctx, fatalSignal)
	stopErr := stopRuntimeContainer(ctx, app, dixRuntime)
	if fatalErr := fatalSignal.Err(); fatalErr != nil && !errors.Is(runErr, fatalErr) {
		runErr = errors.Join(runErr, fatalErr)
	}
	if err := errors.Join(runErr, stopErr); err != nil {
		return oops.In("command").Owner("runtime runner").Wrap(err)
	}
	return nil
}

func waitForRuntimeExit(ctx context.Context, fatalSignal *runtime.FatalSignal) error {
	select {
	case <-ctx.Done():
	case <-fatalSignal.Done():
	}

	select {
	case <-fatalSignal.Done():
		if err := fatalSignal.Err(); err != nil {
			return oops.In("command").Owner("runtime runner").Wrap(err)
		}
		return errors.New("runtime fatal signal closed without an error")
	default:
		return nil
	}
}

func stopRuntimeContainer(ctx context.Context, app *dix.App, dixRuntime *dix.Runtime) error {
	stopCtx := context.WithoutCancel(ctx)
	cancel := func() {}
	if timeout := app.RunStopTimeout(); timeout > 0 {
		stopCtx, cancel = context.WithTimeout(stopCtx, timeout)
	}
	defer cancel()
	if err := dixRuntime.Stop(stopCtx); err != nil {
		return oops.In("command").Owner("runtime runner").Wrap(err)
	}
	return nil
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
