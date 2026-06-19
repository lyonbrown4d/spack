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
)

var Module = dix.NewModule("runtime",
	dix.WithModuleProviders(
		dix.Provider6(newCatalogBootstrapDeps),
		dix.Provider3(newCatalogBootstrapRuntime),
		dix.Provider4(newHTTPRuntime),
		dix.Provider5(newDebugRuntimeDeps),
		dix.Provider3(newDebugRuntime),
	),
	dix.WithModuleHooks(
		dix.OnStart(logConfigOnStart),
		dix.OnStart(bootstrapCatalogOnStart),
		dix.OnStart(startHTTPRuntime),
		dix.OnStart2(startDebugRuntime),
		dix.OnStop(stopDebugRuntime),
		dix.OnStop(stopHTTPRuntime),
	),
)

type catalogBootstrapDeps struct {
	scanner     sourcecatalog.Scanner
	cat         catalog.Catalog
	catMetrics  *catalog.RuntimeMetrics
	bodyCache   *assetcache.Cache
	pipelineSvc *pipeline.Service
	prepared    *server.PreparedService
}

func newCatalogBootstrapDeps(
	scanner sourcecatalog.Scanner,
	cat catalog.Catalog,
	catMetrics *catalog.RuntimeMetrics,
	bodyCache *assetcache.Cache,
	pipelineSvc *pipeline.Service,
	prepared *server.PreparedService,
) catalogBootstrapDeps {
	return catalogBootstrapDeps{
		scanner:     scanner,
		cat:         cat,
		catMetrics:  catMetrics,
		bodyCache:   bodyCache,
		pipelineSvc: pipelineSvc,
		prepared:    prepared,
	}
}

type catalogBootstrapRuntime struct {
	cfg         *config.Config
	scanner     sourcecatalog.Scanner
	cat         catalog.Catalog
	catMetrics  *catalog.RuntimeMetrics
	bodyCache   *assetcache.Cache
	pipelineSvc *pipeline.Service
	prepared    *server.PreparedService
	logger      *slog.Logger
}

func newCatalogBootstrapRuntime(
	cfg *config.Config,
	deps catalogBootstrapDeps,
	logger *slog.Logger,
) catalogBootstrapRuntime {
	return catalogBootstrapRuntime{
		cfg:         cfg,
		scanner:     deps.scanner,
		cat:         deps.cat,
		catMetrics:  deps.catMetrics,
		bodyCache:   deps.bodyCache,
		pipelineSvc: deps.pipelineSvc,
		prepared:    deps.prepared,
		logger:      logger,
	}
}

type httpRuntime struct {
	app    *fiber.App
	cfg    *config.Config
	cat    catalog.Catalog
	logger *slog.Logger
}

func newHTTPRuntime(app *fiber.App, cfg *config.Config, cat catalog.Catalog, logger *slog.Logger) httpRuntime {
	return httpRuntime{
		app:    app,
		cfg:    cfg,
		cat:    cat,
		logger: logger,
	}
}

type debugRuntimeDeps struct {
	pipelineMetrics *pipeline.Metrics
	catMetrics      *catalog.RuntimeMetrics
	serverMetrics   *server.RuntimeMetrics
	taskMetrics     *task.RuntimeMetrics
	asyncMetrics    *asyncx.RuntimeMetrics
}

func newDebugRuntimeDeps(
	pipelineMetrics *pipeline.Metrics,
	catMetrics *catalog.RuntimeMetrics,
	serverMetrics *server.RuntimeMetrics,
	taskMetrics *task.RuntimeMetrics,
	asyncMetrics *asyncx.RuntimeMetrics,
) debugRuntimeDeps {
	return debugRuntimeDeps{
		pipelineMetrics: pipelineMetrics,
		catMetrics:      catMetrics,
		serverMetrics:   serverMetrics,
		taskMetrics:     taskMetrics,
		asyncMetrics:    asyncMetrics,
	}
}

func newDebugRuntime(
	cfg *config.Config,
	logger *slog.Logger,
	deps debugRuntimeDeps,
) *debugRuntime {
	return buildDebugRuntime(cfg, logger, deps)
}
