// Package runtime wires startup lifecycle and long-running services.
package runtime

import (
	"log/slog"

	"github.com/arcgolabs/dix"
	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/pipeline"
	"github.com/lyonbrown4d/spack/internal/server"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/lyonbrown4d/spack/internal/task"
	"github.com/samber/do/v2"
)

var Module = dix.NewModule("runtime",
	dix.WithModuleProviders(
		structProvider[catalogBootstrapRuntime](
			"StructProvider[catalogBootstrapRuntime]",
			dix.TypedService[*config.Config](),
			dix.TypedService[sourcecatalog.Scanner](),
			dix.TypedService[catalog.Catalog](),
			dix.TypedService[*catalog.RuntimeMetrics](),
			dix.TypedService[*assetcache.Cache](),
			dix.TypedService[*pipeline.Service](),
			dix.TypedService[*server.PreparedService](),
			dix.TypedService[*slog.Logger](),
		),
		dix.Provider4(newMainHTTPRuntime),
		dix.Provider6(newCollectorRegistration),
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
	cfg         *config.Config          `do:""`
	scanner     sourcecatalog.Scanner   `do:""`
	cat         catalog.Catalog         `do:""`
	catMetrics  *catalog.RuntimeMetrics `do:""`
	bodyCache   *assetcache.Cache       `do:""`
	pipelineSvc *pipeline.Service       `do:""`
	prepared    *server.PreparedService `do:""`
	logger      *slog.Logger            `do:""`
}

func structProvider[T any](label string, deps ...dix.ServiceRef) dix.ProviderFunc {
	return dix.RawProviderWithMetadata(func(c *dix.Container) {
		do.ProvideNamed[T](c.Raw(), dix.TypedService[T]().Name, func(i do.Injector) (T, error) {
			return do.InvokeStruct[T](i)
		})
	}, dix.ProviderMetadata{
		Label:        label,
		Output:       dix.TypedService[T](),
		Dependencies: dix.ServiceRefs(deps...),
	})
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
	pipelineMetrics *pipeline.Metrics,
	catMetrics *catalog.RuntimeMetrics,
	serverMetrics *server.RuntimeMetrics,
	taskMetrics *task.RuntimeMetrics,
	asyncMetrics *asyncx.RuntimeMetrics,
) *collectorRegistration {
	return buildCollectorRegistration(
		cfg,
		pipelineMetrics,
		catMetrics,
		serverMetrics,
		taskMetrics,
		asyncMetrics,
	)
}
