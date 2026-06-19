package server

import (
	"cmp"
	"context"
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/dix/advanced"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/observabilityx"
	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/pipeline"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/samber/do/v2"
	"log/slog"
)

var Module = dix.NewModule("server",
	dix.WithModuleProviders(
		dix.Provider0(NewRuntimeMetrics),
		dix.Provider2(newResourceHintService),
		dix.Provider5(newPreparedService),
		dix.Provider4(newAssetRouteRuntime),
		dix.Provider2(newHealthCheckDefinitions),
		dix.Provider4(newDiagnosticsRoutesRuntime),
		dix.Contribute1(newMiddlewareRegistration),
		dix.Contribute1(newDiagnosticsRoutesRegistration),
		dix.Contribute2(newHealthRoutesRegistration),
		dix.Contribute1(newRobotsRouteRegistration),
		dix.Contribute1(newAssetRouteRegistration),
		dix.Provider2(newEventPublisher),
		dix.Provider4(newServerFromDeps),
	),
	dix.WithModuleSetups(
		advanced.DoSetupWithMetadata(func(raw do.Injector) error {
			do.ProvideNamed[middlewareRegistrationDeps](
				raw,
				dix.TypedService[middlewareRegistrationDeps]().Name,
				func(i do.Injector) (middlewareRegistrationDeps, error) {
					return do.InvokeStruct[middlewareRegistrationDeps](i)
				},
			)
			return nil
		}, dix.SetupMetadata{
			Label: "MiddlewareRegistrationDepsStruct",
			Dependencies: dix.ServiceRefs(
				dix.TypedService[*config.Config](),
				dix.TypedService[*slog.Logger](),
				dix.TypedService[observabilityx.Observability](),
				dix.TypedService[*RuntimeMetrics](),
			),
			Provides: dix.ServiceRefs(dix.TypedService[middlewareRegistrationDeps]()),
		}),
		advanced.DoSetupWithMetadata(func(raw do.Injector) error {
			do.ProvideNamed[robotsRouteRegistrationDeps](
				raw,
				dix.TypedService[robotsRouteRegistrationDeps]().Name,
				func(i do.Injector) (robotsRouteRegistrationDeps, error) {
					return do.InvokeStruct[robotsRouteRegistrationDeps](i)
				},
			)
			return nil
		}, dix.SetupMetadata{
			Label: "RobotsRouteRegistrationDepsStruct",
			Dependencies: dix.ServiceRefs(
				dix.TypedService[*config.Config](),
				dix.TypedService[*slog.Logger](),
				dix.TypedService[catalog.Catalog](),
				dix.TypedService[*assetcache.Cache](),
			),
			Provides: dix.ServiceRefs(dix.TypedService[robotsRouteRegistrationDeps]()),
		}),
		advanced.DoSetupWithMetadata(func(raw do.Injector) error {
			do.ProvideNamed[assetRouteRegistrationDeps](
				raw,
				dix.TypedService[assetRouteRegistrationDeps]().Name,
				func(i do.Injector) (assetRouteRegistrationDeps, error) {
					return do.InvokeStruct[assetRouteRegistrationDeps](i)
				},
			)
			return nil
		}, dix.SetupMetadata{
			Label: "AssetRouteRegistrationDepsStruct",
			Dependencies: dix.ServiceRefs(
				dix.TypedService[*config.Config](),
				dix.TypedService[assetRouteRuntime](),
				dix.TypedService[*resolver.Resolver](),
				dix.TypedService[*pipeline.Service](),
				dix.TypedService[*assetcache.Cache](),
				dix.TypedService[eventx.BusRuntime](),
			),
			Provides: dix.ServiceRefs(dix.TypedService[assetRouteRegistrationDeps]()),
		}),
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
	cfg     *config.Config               `do:""`
	logger  *slog.Logger                 `do:""`
	obs     observabilityx.Observability `do:""`
	metrics *RuntimeMetrics              `do:""`
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
	cfg       *config.Config    `do:""`
	logger    *slog.Logger      `do:""`
	cat       catalog.Catalog   `do:""`
	bodyCache *assetcache.Cache `do:""`
}

func newRobotsRouteRegistration(deps robotsRouteRegistrationDeps) appRegistration {
	return newAppRegistration(250, "robots_route", func(app *fiber.App) {
		registerRobotsRoute(app, deps.cfg, deps.logger, deps.cat, deps.bodyCache)
	})
}

type assetRouteRegistrationDeps struct {
	cfg           *config.Config     `do:""`
	runtime       assetRouteRuntime  `do:""`
	assetResolver *resolver.Resolver `do:""`
	pipelineSvc   *pipeline.Service  `do:""`
	bodyCache     *assetcache.Cache  `do:""`
	bus           eventx.BusRuntime  `do:""`
}

func newAssetRouteRegistration(deps assetRouteRegistrationDeps) appRegistration {
	return newAppRegistration(300, "asset_route", func(app *fiber.App) {
		registerAssetRoute(app, newAssetDeliveryRuntime(
			deps.cfg,
			deps.runtime,
			deps.assetResolver,
			deps.pipelineSvc,
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
