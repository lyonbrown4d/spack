package server

import (
	"context"
	"log/slog"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/observabilityx"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/requestpath"
	"github.com/lyonbrown4d/spack/internal/resolver"
)

type PreparedSelectionForTest struct {
	FilePath     string
	Encoding     string
	BodyLen      int
	FallbackUsed bool
}

type ResourceHintEntryForTest struct {
	Header string
	Len    int
}

type ResourceHintServiceForTest struct {
	service *resourceHintService
}

// ShouldVaryAcceptForTest exposes vary-header behavior for external tests.
func ShouldVaryAcceptForTest(sourceMediaType, explicitFormat string) bool {
	return shouldVaryAccept(sourceMediaType, explicitFormat)
}

// IdentityHeaderMiddlewareForTest exposes identity-header stripping behavior for external tests.
func IdentityHeaderMiddlewareForTest(cfg *config.Config) fiber.Handler {
	return identityHeaderMiddleware(cfg)
}

// ServerHeaderForTest exposes configured server-header construction for external tests.
func ServerHeaderForTest(cfg *config.Config, meta dix.AppMeta) string {
	return serverHeader(cfg, meta)
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

func NewResourceHintServiceForTest(
	cfg *config.Config,
	logger *slog.Logger,
) *ResourceHintServiceForTest {
	return &ResourceHintServiceForTest{service: newResourceHintService(cfg, logger, nil)}
}

func (s *ResourceHintServiceForTest) Entry(result *resolver.Result) (ResourceHintEntryForTest, bool) {
	if s == nil || s.service == nil {
		return ResourceHintEntryForTest{}, false
	}
	entry, ok := s.service.Entry(result)
	if !ok {
		return ResourceHintEntryForTest{}, false
	}
	linkCount := 0
	if entry.links != nil {
		linkCount = entry.links.Len()
	}
	return ResourceHintEntryForTest{
		Header: entry.header,
		Len:    linkCount,
	}, true
}

// NewAppForTest exposes server construction for external tests.
func NewAppForTest(
	cfg *config.Config,
	logger *slog.Logger,
	cat catalog.Catalog,
	bodyCache *assetcache.Cache,
	assetResolver *resolver.Resolver,
	bus eventx.BusRuntime,
) *fiber.App {
	return NewObservedAppForTest(cfg, logger, nil, nil, cat, bodyCache, assetResolver, bus)
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
	bus eventx.BusRuntime,
) *fiber.App {
	return newObservedAppForTest(cfg, logger, obs, runtimeMetrics, cat, bodyCache, assetResolver, bus, nil)
}

func NewPreparedAppForTest(
	cfg *config.Config,
	logger *slog.Logger,
	cat catalog.Catalog,
	bodyCache *assetcache.Cache,
	assetResolver *resolver.Resolver,
	bus eventx.BusRuntime,
) (*fiber.App, error) {
	prepared := NewPreparedServiceForTest(cfg, logger, cat)
	if err := prepared.Rebuild(context.Background()); err != nil {
		return nil, err
	}
	return newObservedAppForTest(cfg, logger, nil, nil, cat, bodyCache, assetResolver, bus, prepared), nil
}

func newObservedAppForTest(
	cfg *config.Config,
	logger *slog.Logger,
	obs observabilityx.Observability,
	runtimeMetrics *RuntimeMetrics,
	cat catalog.Catalog,
	bodyCache *assetcache.Cache,
	assetResolver *resolver.Resolver,
	bus eventx.BusRuntime,
	prepared *PreparedService,
) *fiber.App {
	healthChecks := newHealthCheckDefinitions(cfg, cat)
	return newServerFromDeps(cfg, dix.AppMeta{Version: "test"}, logger, newServerRegistrations(
		cxlist.NewList[appRegistration](
			newMiddlewareRegistration(middlewareRegistrationDeps{
				cfg:     cfg,
				logger:  logger,
				obs:     obs,
				metrics: runtimeMetrics,
			}),
			newDiagnosticsRoutesRegistration(newDiagnosticsRoutesRuntime(cfg, logger, nil, cat)),
			newHealthRoutesRegistration(healthChecks, obs),
			newRobotsRouteRegistration(robotsRouteRegistrationDeps{
				cfg:       cfg,
				logger:    logger,
				cat:       cat,
				bodyCache: bodyCache,
			}),
			newAssetRouteRegistration(assetRouteRegistrationDeps{
				cfg:           cfg,
				runtime:       newAssetRouteRuntime(logger, obs, newResourceHintService(cfg, logger, nil), prepared, nil),
				assetResolver: assetResolver,
				bodyCache:     bodyCache,
				bus:           bus,
				cat:           cat,
			}),
		),
	))
}

func NewPreparedServiceForTest(
	cfg *config.Config,
	logger *slog.Logger,
	cat catalog.Catalog,
) *PreparedService {
	return newPreparedService(cfg, cat, logger, nil, nil, nil)
}

func NewPreparedServiceWithRuntimeMetricsForTest(
	cfg *config.Config,
	logger *slog.Logger,
	cat catalog.Catalog,
	metrics *RuntimeMetrics,
) *PreparedService {
	return newPreparedService(cfg, cat, logger, nil, metrics, nil)
}
func ResolvePreparedForTest(
	svc *PreparedService,
	request resolver.Request,
	requestedFormat string,
) (PreparedSelectionForTest, bool) {
	return ResolvePreparedCleanedForTest(svc, request, requestedFormat, requestpath.Clean(request.Path))
}

func ResolvePreparedCleanedForTest(
	svc *PreparedService,
	request resolver.Request,
	requestedFormat string,
	cleanedPath requestpath.Cleaned,
) (PreparedSelectionForTest, bool) {
	selection, ok := svc.Resolve(preparedRequest{
		Request:         request,
		RequestedFormat: requestedFormat,
		CleanedPath:     cleanedPath,
	}).Get()
	if !ok || selection.response == nil {
		return PreparedSelectionForTest{}, false
	}
	response := selection.response
	out := PreparedSelectionForTest{
		FilePath:     response.filePath(),
		FallbackUsed: selection.fallbackUsed,
	}
	if response.bodyPrepared {
		out.BodyLen = len(response.body)
	}
	if variant := response.variant(); variant != nil {
		out.Encoding = variant.Encoding
	}
	return out, true
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
