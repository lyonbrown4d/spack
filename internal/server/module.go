package server

import (
	"cmp"
	"context"
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/observabilityx"
	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"log/slog"
)

var Module = dix.NewModule("server",
	dix.WithModuleProviders(
		dix.Provider0(NewRuntimeMetrics),
		dix.Provider2(newResourceHintService),
		dix.Provider6(newPreparedService),
		dix.Provider4(newAssetRouteRuntime),
		dix.Provider2(newHealthCheckDefinitions),
		dix.Provider4(newDiagnosticsRoutesRuntime),
		dix.Provider4(newMiddlewareRegistrationDeps),
		dix.Provider4(newRobotsRouteRegistrationDeps),
		dix.Provider5(newAssetRouteRegistrationDeps),
		dix.Contribute1(newMiddlewareRegistration),
		dix.Contribute1(newDiagnosticsRoutesRegistration),
		dix.Contribute2(newHealthRoutesRegistration),
		dix.Contribute1(newRobotsRouteRegistration),
		dix.Contribute1(newAssetRouteRegistration),
		dix.Provider2(newEventPublisher),
		dix.Provider4(newServerFromDeps),
	),
	dix.WithModuleSetups(
		dix.Setup(registerHealthCheckSetup),
	),
	dix.WithModuleHooks(
		dix.OnStart(func(ctx context.Context, svc *PreparedService) error {
			return svc.start(ctx)
		}),
		dix.OnStop(func(ctx context.Context, svc *PreparedService) error {
			return svc.stop(ctx)
		}),
		dix.OnStop(stopDiagnosticsRoutesRuntime),
	),
)

var CoreModule = dix.NewModule("server_core",
	dix.WithModuleProviders(
		dix.Provider0(NewRuntimeMetrics),
		dix.Provider2(newResourceHintService),
		dix.Provider6(newPreparedService),
		dix.Provider2(newEventPublisher),
		dix.Provider4(newServerFromDeps),
	),
	dix.WithModuleHooks(
		dix.OnStart(func(ctx context.Context, svc *PreparedService) error {
			return svc.start(ctx)
		}),
		dix.OnStop(func(ctx context.Context, svc *PreparedService) error {
			return svc.stop(ctx)
		}),
	),
)

var MiddlewareModule = dix.NewModule("server_middleware",
	dix.WithModuleProviders(
		dix.Provider4(newMiddlewareRegistrationDeps),
		dix.Contribute1(newMiddlewareRegistration),
	),
)

var DiagnosticsModule = dix.NewModule("server_diagnostics",
	dix.WithModuleProviders(
		dix.Provider4(newDiagnosticsRoutesRuntime),
		dix.Contribute1(newDiagnosticsRoutesRegistration),
	),
	dix.WithModuleHooks(
		dix.OnStop(stopDiagnosticsRoutesRuntime),
	),
)

var HealthModule = dix.NewModule("server_health",
	dix.WithModuleProviders(
		dix.Provider2(newHealthCheckDefinitions),
		dix.Contribute2(newHealthRoutesRegistration),
	),
	dix.WithModuleSetups(
		dix.Setup(registerHealthCheckSetup),
	),
)

var RobotsModule = dix.NewModule("server_robots",
	dix.WithModuleProviders(
		dix.Provider4(newRobotsRouteRegistrationDeps),
		dix.Contribute1(newRobotsRouteRegistration),
	),
)

var AssetsModule = dix.NewModule("server_assets",
	dix.WithModuleProviders(
		dix.Provider4(newAssetRouteRuntime),
		dix.Provider5(newAssetRouteRegistrationDeps),
		dix.Contribute1(newAssetRouteRegistration),
	),
)

func Modules() []dix.Module {
	return []dix.Module{
		CoreModule,
		MiddlewareModule,
		DiagnosticsModule,
		HealthModule,
		RobotsModule,
		AssetsModule,
	}
}

type appRegistration struct {
	Order int
	Name  string
	Apply func(*fiber.App)
}

type assetRouteRuntime struct {
	logger        *slog.Logger
	trackDelivery bool
	resourceHints *resourceHintService
	prepared      *PreparedService
}

func newAppRegistration(order int, name string, apply func(*fiber.App)) appRegistration {
	return appRegistration{
		Order: order,
		Name:  name,
		Apply: apply,
	}
}

type middlewareRegistrationDeps struct {
	cfg     *config.Config
	logger  *slog.Logger
	obs     observabilityx.Observability
	metrics *RuntimeMetrics
}

func newMiddlewareRegistrationDeps(
	cfg *config.Config,
	logger *slog.Logger,
	obs observabilityx.Observability,
	metrics *RuntimeMetrics,
) middlewareRegistrationDeps {
	return middlewareRegistrationDeps{
		cfg:     cfg,
		logger:  logger,
		obs:     obs,
		metrics: metrics,
	}
}

func newMiddlewareRegistration(deps middlewareRegistrationDeps) appRegistration {
	return newAppRegistration(100, "middleware", func(app *fiber.App) {
		registerMiddleware(app, deps.cfg, deps.logger, deps.obs, deps.metrics)
	})
}

func newDiagnosticsRoutesRegistration(runtime *diagnosticsRoutesRuntime) appRegistration {
	return newAppRegistration(150, "diagnostics_routes", func(app *fiber.App) {
		registerDiagnosticsRoutes(app, runtime)
	})
}

func newHealthRoutesRegistration(
	checks *cxlist.List[healthCheckDefinition],
	obs observabilityx.Observability,
) appRegistration {
	return newAppRegistration(200, "health_routes", func(app *fiber.App) {
		registerHealthRoutes(app, checks, obs)
	})
}

type robotsRouteRegistrationDeps struct {
	cfg       *config.Config
	logger    *slog.Logger
	cat       catalog.Catalog
	bodyCache *assetcache.Cache
}

func newRobotsRouteRegistrationDeps(
	cfg *config.Config,
	logger *slog.Logger,
	cat catalog.Catalog,
	bodyCache *assetcache.Cache,
) robotsRouteRegistrationDeps {
	return robotsRouteRegistrationDeps{
		cfg:       cfg,
		logger:    logger,
		cat:       cat,
		bodyCache: bodyCache,
	}
}

func newRobotsRouteRegistration(deps robotsRouteRegistrationDeps) appRegistration {
	return newAppRegistration(250, "robots_route", func(app *fiber.App) {
		registerRobotsRoute(app, deps.cfg, deps.logger, deps.cat, deps.bodyCache)
	})
}

type assetRouteRegistrationDeps struct {
	cfg           *config.Config
	runtime       assetRouteRuntime
	assetResolver *resolver.Resolver
	bodyCache     *assetcache.Cache
	bus           eventx.BusRuntime
}

func newAssetRouteRegistrationDeps(
	cfg *config.Config,
	runtime assetRouteRuntime,
	assetResolver *resolver.Resolver,
	bodyCache *assetcache.Cache,
	bus eventx.BusRuntime,
) assetRouteRegistrationDeps {
	return assetRouteRegistrationDeps{
		cfg:           cfg,
		runtime:       runtime,
		assetResolver: assetResolver,
		bodyCache:     bodyCache,
		bus:           bus,
	}
}

func newAssetRouteRegistration(deps assetRouteRegistrationDeps) appRegistration {
	return newAppRegistration(300, "asset_route", func(app *fiber.App) {
		registerAssetRoute(app, newAssetDeliveryRuntime(
			deps.cfg,
			deps.runtime,
			deps.assetResolver,
			deps.bodyCache,
			deps.bus,
		))
	})
}

func newAssetRouteRuntime(
	logger *slog.Logger,
	obs observabilityx.Observability,
	resourceHints *resourceHintService,
	prepared *PreparedService,
) assetRouteRuntime {
	return assetRouteRuntime{
		logger:        logger,
		trackDelivery: shouldTrackAssetDelivery(logger, obs),
		resourceHints: resourceHints,
		prepared:      prepared,
	}
}

func shouldTrackAssetDelivery(logger *slog.Logger, obs observabilityx.Observability) bool {
	return obs != nil || (logger != nil && logger.Enabled(context.Background(), slog.LevelInfo))
}

func newServerRegistrations(
	registrations *cxlist.List[appRegistration],
) *cxlist.List[appRegistration] {
	return registrations.Clone().Sort(func(left, right appRegistration) int {
		if left.Order != right.Order {
			return cmp.Compare(left.Order, right.Order)
		}
		return cmp.Compare(left.Name, right.Name)
	})
}

func newServerFromDeps(
	cfg *config.Config,
	meta dix.AppMeta,
	logger *slog.Logger,
	registrations *cxlist.List[appRegistration],
) *fiber.App {
	app := newServerApp(cfg, meta, logger)
	newServerRegistrations(registrations).
		Where(func(_ int, registration appRegistration) bool {
			return registration.Apply != nil
		}).
		Each(func(_ int, registration appRegistration) {
			registration.Apply(app)
		})
	return app
}
