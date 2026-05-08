package server

import (
	"context"
	"log/slog"

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
	"github.com/gofiber/fiber/v3/middleware/helmet"
)

// ShouldVaryAcceptForTest exposes vary-header behavior for external tests.
func ShouldVaryAcceptForTest(sourceMediaType, explicitFormat string) bool {
	return shouldVaryAccept(sourceMediaType, explicitFormat)
}

// MetricsMiddlewareForTest exposes HTTP metrics middleware for external tests.
func MetricsMiddlewareForTest(obs observabilityx.Observability) fiber.Handler {
	return metricsMiddleware(obs, nil)
}

// MetricsMiddlewareWithRuntimeMetricsForTest exposes HTTP metrics middleware with runtime gauges for external tests.
func MetricsMiddlewareWithRuntimeMetricsForTest(obs observabilityx.Observability, runtimeMetrics *RuntimeMetrics) fiber.Handler {
	return metricsMiddleware(obs, runtimeMetrics)
}

// SetAssetDeliveryForTest exposes delivery tagging for external tests.
func SetAssetDeliveryForTest(c fiber.Ctx, delivery string) {
	setAssetDelivery(c, delivery)
}

// PublishVariantServedForTest exposes variant-served event publishing for external tests.
func PublishVariantServedForTest(
	ctx context.Context,
	result *resolver.Result,
	bus eventx.BusRuntime,
	logger *slog.Logger,
) {
	publishVariantServed(ctx, result, bus, logger)
}

// NewAppForTest exposes server construction for external tests.
func NewAppForTest(
	cfg *config.Config,
	logger *slog.Logger,
	cat catalog.Catalog,
	bodyCache *assetcache.Cache,
	assetResolver *resolver.Resolver,
	pipelineSvc *pipeline.Service,
	bus eventx.BusRuntime,
) *fiber.App {
	return NewObservedAppForTest(cfg, logger, nil, nil, cat, bodyCache, assetResolver, pipelineSvc, bus)
}

// NewObservedAppForTest exposes server construction with observability and runtime metrics for external tests.
func NewObservedAppForTest(
	cfg *config.Config,
	logger *slog.Logger,
	obs observabilityx.Observability,
	runtimeMetrics *RuntimeMetrics,
	cat catalog.Catalog,
	bodyCache *assetcache.Cache,
	assetResolver *resolver.Resolver,
	pipelineSvc *pipeline.Service,
	bus eventx.BusRuntime,
) *fiber.App {
	healthChecks := newHealthCheckDefinitions(cfg, cat)
	return newServerFromDeps(cfg, dix.AppMeta{Version: "test"}, newServerRegistrations(
		cxlist.NewList[appRegistration](
			newMiddlewareRegistration(newMiddlewareRegistrationDeps(cfg, logger, obs, runtimeMetrics)),
			newHealthRoutesRegistration(newHealthRoutesRegistrationDeps(cat, healthChecks, obs)),
			newRobotsRouteRegistration(newRobotsRouteRegistrationDeps(cfg, logger, cat, bodyCache)),
			newAssetRouteRegistration(newAssetRouteRegistrationDeps(
				cfg,
				newAssetRouteRuntime(logger, obs, newResourceHintService(&cfg.Frontend, logger)),
				assetResolver,
				pipelineSvc,
				bodyCache,
				bus,
			)),
		),
	))
}

// NewHelmetConfigForTest exposes the helmet configuration for external tests.
func NewHelmetConfigForTest() helmet.Config {
	return newHelmetConfig()
}

// NewHealthModuleForTest exposes the dix health-check setup for external tests.
func NewHealthModuleForTest(cfg *config.Config, cat catalog.Catalog) dix.Module {
	return dix.NewModule("server_health_test",
		dix.WithModuleProviders(
			dix.Provider0(func() *config.Config { return cfg }),
			dix.Provider0(func() catalog.Catalog { return cat }),
			dix.Provider2(newHealthCheckDefinitions),
		),
		dix.WithModuleSetups(
			dix.Setup(registerHealthCheckSetup),
		),
	)
}
