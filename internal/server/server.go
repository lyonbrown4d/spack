package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/observabilityx"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/etag"
	expvarmw "github.com/gofiber/fiber/v3/middleware/expvar"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	recoverer "github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/gofiber/fiber/v3/middleware/responsetime"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/media"
	"github.com/samber/lo"
	"github.com/samber/oops"
	slogfiber "github.com/samber/slog-fiber"
)

var (
	httpRequestsTotalSpec = observabilityx.NewCounterSpec(
		"http_requests_total",
		observabilityx.WithDescription("Total number of HTTP requests handled by the Fiber server."),
		observabilityx.WithLabelKeys("method", "route", "status"),
	)
	httpRequestDurationSpec = observabilityx.NewHistogramSpec(
		"http_request_duration_seconds",
		observabilityx.WithDescription("HTTP request duration in seconds."),
		observabilityx.WithUnit("s"),
		observabilityx.WithLabelKeys("method", "route", "status"),
	)
	httpAssetDeliveryTotalSpec = observabilityx.NewCounterSpec(
		"http_asset_delivery_total",
		observabilityx.WithDescription("Total number of asset delivery responses by delivery path."),
		observabilityx.WithLabelKeys("method", "route", "status", "delivery"),
	)
	httpAssetDeliveryDurationSpec = observabilityx.NewHistogramSpec(
		"http_asset_delivery_duration_seconds",
		observabilityx.WithDescription("Asset delivery request duration in seconds."),
		observabilityx.WithUnit("s"),
		observabilityx.WithLabelKeys("method", "route", "status", "delivery"),
	)
)

func newServerApp(cfg *config.Config, meta dix.AppMeta, logger *slog.Logger) *fiber.App {
	return fiber.New(fiber.Config{
		AppName:           "Spack",
		Immutable:         true,
		StreamRequestBody: true,
		UnescapePath:      true,
		ErrorHandler:      newErrorHandler(logger),
		ServerHeader:      serverHeader(cfg, meta),
		StrictRouting:     true,
		ReduceMemoryUsage: cfg.HTTP.LowMemory,
	})
}

func serverHeader(cfg *config.Config, meta dix.AppMeta) string {
	if cfg == nil || !cfg.HTTP.ExposeServerHeader {
		return ""
	}
	return buildServerHeader(cfg, meta)
}

func registerMiddleware(
	app *fiber.App,
	cfg *config.Config,
	logger *slog.Logger,
	obs observabilityx.Observability,
	runtimeMetrics *RuntimeMetrics,
) {
	app.Use(identityHeaderMiddleware(cfg))
	app.Use(requestContextMiddleware())
	requestIDConfig := requestid.ConfigDefault
	requestIDConfig.Header = RequestIDHeader
	app.Use(requestid.New(requestIDConfig))
	app.Use(etag.New())
	app.Use(helmet.New(newHelmetConfig()))
	requestLogMiddleware(app, logger, cfg)
	if metrics := metricsMiddleware(obs, runtimeMetrics); metrics != nil {
		app.Use(metrics)
	}
	app.Use(responsetime.New(responsetime.Config{
		Header: "X-Elapsed",
	}))
	if cfg.Debug.Enable {
		app.Use(expvarmw.New())
	}

	recoverConfig := recoverer.ConfigDefault
	recoverConfig.EnableStackTrace = true
	app.Use(recoverer.New(recoverConfig))
}

func requestContextMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		parent := c.Context()
		if parent == nil {
			parent = context.Background()
		}
		ctx, cancel := context.WithCancel(parent)
		c.SetContext(ctx)
		defer cancel()
		if err := c.Next(); err != nil {
			return oops.In("server").Wrap(fmt.Errorf("run request context middleware chain: %w", err))
		}
		return nil
	}
}
func identityHeaderMiddleware(cfg *config.Config) fiber.Handler {
	stripServerHeader := cfg == nil || !cfg.HTTP.ExposeServerHeader
	return func(c fiber.Ctx) error {
		if err := c.Next(); err != nil {
			return oops.In("server").Wrap(fmt.Errorf("strip identity headers: %w", err))
		}
		c.Response().Header.Del(poweredByHeader)
		if stripServerHeader {
			c.Response().Header.Del(fiber.HeaderServer)
		}
		return nil
	}
}

func metricsMiddleware(obs observabilityx.Observability, runtimeMetrics *RuntimeMetrics) fiber.Handler {
	if obs == nil && runtimeMetrics == nil {
		return nil
	}
	if obs == nil {
		return runtimeInFlightMiddleware(runtimeMetrics)
	}

	recorder := newHTTPMetricsRecorder(obs)
	return func(c fiber.Ctx) error {
		runtimeMetrics.IncRequestsInFlight()
		defer runtimeMetrics.DecRequestsInFlight()

		startedAt := time.Now()
		err := c.Next()
		recorder.Record(c, finalHTTPStatus(c, err), time.Since(startedAt).Seconds())
		return wrapMetricsMiddlewareError(err)
	}
}

type httpMetricsRecorder struct {
	requestCounter        observabilityx.Counter
	requestDuration       observabilityx.Histogram
	assetDeliveryCounter  observabilityx.Counter
	assetDeliveryDuration observabilityx.Histogram
}

func newHTTPMetricsRecorder(obs observabilityx.Observability) httpMetricsRecorder {
	return httpMetricsRecorder{
		requestCounter:        obs.Counter(httpRequestsTotalSpec),
		requestDuration:       obs.Histogram(httpRequestDurationSpec),
		assetDeliveryCounter:  obs.Counter(httpAssetDeliveryTotalSpec),
		assetDeliveryDuration: obs.Histogram(httpAssetDeliveryDurationSpec),
	}
}

func runtimeInFlightMiddleware(runtimeMetrics *RuntimeMetrics) fiber.Handler {
	return func(c fiber.Ctx) error {
		runtimeMetrics.IncRequestsInFlight()
		defer runtimeMetrics.DecRequestsInFlight()
		return wrapMetricsMiddlewareError(c.Next())
	}
}

func (r httpMetricsRecorder) Record(c fiber.Ctx, status int, duration float64) {
	ctx := fiberRequestContext(c)
	requestAttrs := requestMetricsAttrs(c, status)
	r.requestCounter.Add(ctx, 1, requestAttrs...)
	r.requestDuration.Record(ctx, duration, requestAttrs...)

	deliveryAttrs := assetDeliveryMetricsAttrs(c)
	if len(deliveryAttrs) == 0 {
		return
	}
	r.assetDeliveryCounter.Add(ctx, 1, deliveryAttrs...)
	r.assetDeliveryDuration.Record(ctx, duration, deliveryAttrs...)
}

func fiberRequestContext(c fiber.Ctx) context.Context {
	ctx := c.Context()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func wrapMetricsMiddlewareError(err error) error {
	if err == nil {
		return nil
	}
	return oops.In("server").Wrap(fmt.Errorf("run metrics middleware chain: %w", err))
}
func requestMetricsAttrs(c fiber.Ctx, status int) []observabilityx.Attribute {
	return []observabilityx.Attribute{
		observabilityx.String("method", c.Method()),
		observabilityx.String("route", requestRoute(c)),
		observabilityx.String("status", strconv.Itoa(status)),
	}
}

func finalHTTPStatus(c fiber.Ctx, err error) int {
	if err == nil {
		status := c.Response().StatusCode()
		if status == 0 {
			return fiber.StatusOK
		}
		return status
	}
	if fiberErr, ok := errors.AsType[*fiber.Error](err); ok {
		return fiberErr.Code
	}
	return fiber.StatusInternalServerError
}

func assetDeliveryMetricsAttrs(c fiber.Ctx) []observabilityx.Attribute {
	delivery := getAssetDelivery(c)
	if delivery == "" {
		return nil
	}
	return lo.Concat(requestMetricsAttrs(c, c.Response().StatusCode()), []observabilityx.Attribute{observabilityx.String("delivery", delivery)})
}

func requestRoute(c fiber.Ctx) string {
	route := c.Route()
	if route != nil {
		if path := strings.TrimSpace(route.Path); path != "" {
			return path
		}
	}
	return "unmatched"
}

func requestLogMiddleware(app *fiber.App, logger *slog.Logger, cfg *config.Config) {
	fiberslogcfg := slogfiber.Config{
		WithSpanID:         true,
		WithTraceID:        true,
		WithRequestHeader:  cfg.HTTP.RequestLogDetail,
		WithResponseHeader: cfg.HTTP.RequestLogDetail,
	}
	app.Use(slogfiber.NewWithConfig(logger.WithGroup("server"), fiberslogcfg))
}

func shouldVaryAccept(sourceMediaType, explicitFormat string) bool {
	if strings.TrimSpace(explicitFormat) != "" {
		return false
	}
	return media.IsImageMediaType(sourceMediaType)
}
