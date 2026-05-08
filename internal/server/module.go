package server

import (
	"cmp"
	"context"
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/observabilityx"
	"github.com/daiyuang/spack/internal/assetcache"
	"github.com/daiyuang/spack/internal/catalog"
	"github.com/daiyuang/spack/internal/config"
	"github.com/daiyuang/spack/internal/pipeline"
	"github.com/daiyuang/spack/internal/resolver"
	"github.com/gofiber/fiber/v3"
	"log/slog"
)

var Module = dix.NewModule("server",
	dix.WithModuleProviders(
		dix.Provider0(NewRuntimeMetrics),
		dix.Provider2(newResourceHintService),
		dix.Provider3(newAssetRouteRuntime),
		dix.Provider2(newHealthCheckDefinitions),
		dix.Provider4(newMiddlewareRegistrationDeps),
		dix.Provider3(newHealthRoutesRegistrationDeps),
		dix.Provider4(newRobotsRouteRegistrationDeps),
		dix.Provider6(newAssetRouteRegistrationDeps),
		dix.Contribute1(newMiddlewareRegistration),
		dix.Contribute1(newHealthRoutesRegistration),
		dix.Contribute1(newRobotsRouteRegistration),
		dix.Contribute1(newAssetRouteRegistration),
		dix.Provider1(newServerRegistrations),
		dix.Provider2(newEventPublisher),
		dix.Provider3(newServerFromDeps),
	),
	dix.WithModuleSetups(
		dix.Setup(registerHealthCheckSetup),
	),
)

type appRegistration struct {
	Order int
	Name  string
	Apply func(*fiber.App)
}

type assetRouteRuntime struct {
	logger        *slog.Logger
	trackDelivery bool
	resourceHints *resourceHintService
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
	runtimeMetrics *RuntimeMetrics,
) middlewareRegistrationDeps {
	return middlewareRegistrationDeps{
		cfg:     cfg,
		logger:  logger,
		obs:     obs,
		metrics: runtimeMetrics,
	}
}

func newMiddlewareRegistration(deps middlewareRegistrationDeps) appRegistration {
	return newAppRegistration(100, "middleware", func(app *fiber.App) {
		registerMiddleware(app, deps.cfg, deps.logger, deps.obs, deps.metrics)
	})
}

type healthRoutesRegistrationDeps struct {
	cat    catalog.Catalog
	checks *cxlist.List[healthCheckDefinition]
	obs    observabilityx.Observability
}

func newHealthRoutesRegistrationDeps(
	cat catalog.Catalog,
	checks *cxlist.List[healthCheckDefinition],
	obs observabilityx.Observability,
) healthRoutesRegistrationDeps {
	return healthRoutesRegistrationDeps{
		cat:    cat,
		checks: checks,
		obs:    obs,
	}
}

func newHealthRoutesRegistration(deps healthRoutesRegistrationDeps) appRegistration {
	return newAppRegistration(200, "health_routes", func(app *fiber.App) {
		registerHealthRoutes(app, deps.cat, deps.checks, deps.obs)
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
	pipelineSvc   *pipeline.Service
	bodyCache     *assetcache.Cache
	bus           eventx.BusRuntime
}

func newAssetRouteRegistrationDeps(
	cfg *config.Config,
	runtime assetRouteRuntime,
	assetResolver *resolver.Resolver,
	pipelineSvc *pipeline.Service,
	bodyCache *assetcache.Cache,
	bus eventx.BusRuntime,
) assetRouteRegistrationDeps {
	return assetRouteRegistrationDeps{
		cfg:           cfg,
		runtime:       runtime,
		assetResolver: assetResolver,
		pipelineSvc:   pipelineSvc,
		bodyCache:     bodyCache,
		bus:           bus,
	}
}

func newAssetRouteRegistration(deps assetRouteRegistrationDeps) appRegistration {
	return newAppRegistration(300, "asset_route", func(app *fiber.App) {
		registerAssetRoute(app, newAssetDeliveryRuntime(assetDeliveryRuntimeDeps{
			cfg:           deps.cfg,
			routeRuntime:  deps.runtime,
			assetResolver: deps.assetResolver,
			pipelineSvc:   deps.pipelineSvc,
			bodyCache:     deps.bodyCache,
			bus:           deps.bus,
		}))
	})
}

func newAssetRouteRuntime(logger *slog.Logger, obs observabilityx.Observability, resourceHints *resourceHintService) assetRouteRuntime {
	return assetRouteRuntime{
		logger:        logger,
		trackDelivery: shouldTrackAssetDelivery(logger, obs),
		resourceHints: resourceHints,
	}
}

func shouldTrackAssetDelivery(logger *slog.Logger, obs observabilityx.Observability) bool {
	return obs != nil || (logger != nil && logger.Enabled(context.Background(), slog.LevelInfo))
}

func newServerRegistrations(
	registrations *cxlist.List[appRegistration],
) *cxlist.List[appRegistration] {
	return registrations.Sort(func(left, right appRegistration) int {
		if left.Order != right.Order {
			return cmp.Compare(left.Order, right.Order)
		}
		return cmp.Compare(left.Name, right.Name)
	})
}

func newServerFromDeps(
	cfg *config.Config,
	meta dix.AppMeta,
	registrations *cxlist.List[appRegistration],
) *fiber.App {
	app := newServerApp(cfg, meta)
	registrations.Range(func(_ int, registration appRegistration) bool {
		if registration.Apply != nil {
			registration.Apply(app)
		}
		return true
	})
	return app
}
