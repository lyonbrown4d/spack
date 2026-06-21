// Package runtime wires startup lifecycle and long-running services.
package runtime

import (
	"log/slog"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/dix/advanced"
	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/server"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/lyonbrown4d/spack/internal/task"
	"github.com/samber/do/v2"
)

var Module = dix.NewModule("runtime",
	dix.WithModuleProviders(
		dix.Provider4(newMainHTTPRuntime),
		dix.Provider5(newCollectorRegistration),
	),
	dix.Setups(
		advanced.DoSetupWithMetadata(func(raw do.Injector) error {
			do.ProvideNamed[catalogBootstrapRuntime](raw, dix.TypedService[catalogBootstrapRuntime]().Name, func(i do.Injector) (catalogBootstrapRuntime, error) {
				return do.InvokeStruct[catalogBootstrapRuntime](i)
			})
			return nil
		}, dix.SetupMetadata{
			Label: "CatalogBootstrapRuntimeStruct",
			Dependencies: dix.ServiceRefs(
				dix.TypedService[*config.Config](),
				dix.TypedService[sourcecatalog.Scanner](),
				dix.TypedService[catalog.Catalog](),
				dix.TypedService[*catalog.RuntimeMetrics](),
				dix.TypedService[*assetcache.Cache](),
				dix.TypedService[*server.PreparedService](),
				dix.TypedService[*slog.Logger](),
			),
			Provides: dix.ServiceRefs(
				dix.TypedService[catalogBootstrapRuntime](),
			),
			GraphMutation: true,
		}),
	),
	dix.WithModuleHooks(
		dix.OnStart(logConfigOnStart),
		dix.OnStart(bootstrapCatalogOnStart),
		dix.OnStart2(startRuntimeCollectors),
		dix.OnStart(startMainHTTPRuntime),
		dix.OnStop(stopMainHTTPRuntime),
	),
)

type catalogBootstrapRuntime struct {
	cfg        *config.Config          `do:""`
	scanner    sourcecatalog.Scanner   `do:""`
	cat        catalog.Catalog         `do:""`
	catMetrics *catalog.RuntimeMetrics `do:""`
	bodyCache  *assetcache.Cache       `do:""`
	prepared   *server.PreparedService `do:""`
	logger     *slog.Logger            `do:""`
}

type mainHTTPRuntime struct {
	app    *fiber.App
	cfg    *config.Config
	cat    catalog.Catalog
	logger *slog.Logger
}

func newMainHTTPRuntime(app *fiber.App, cfg *config.Config, cat catalog.Catalog, logger *slog.Logger) mainHTTPRuntime {
	return mainHTTPRuntime{
		app:    app,
		cfg:    cfg,
		cat:    cat,
		logger: logger,
	}
}

func newCollectorRegistration(
	cfg *config.Config,
	catMetrics *catalog.RuntimeMetrics,
	serverMetrics *server.RuntimeMetrics,
	taskMetrics *task.RuntimeMetrics,
	asyncMetrics *asyncx.RuntimeMetrics,
) *collectorRegistration {
	return buildCollectorRegistration(
		cfg,
		catMetrics,
		serverMetrics,
		taskMetrics,
		asyncMetrics,
	)
}
