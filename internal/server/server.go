package server

import (
	"context"
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
	"github.com/samber/oops"
	slogfiber "github.com/samber/slog-fiber"
)

var (
	httpRequestsTotalSpec = observabilityx.NewCounterSpec(
		"http_requests_total",
		observabilityx.WithDescription("Total number of HTTP requests handled by the Fiber server."),
		observabilityx.WithLabelKeys("method", "path", "status"),
	)
	httpRequestDurationSpec = observabilityx.NewHistogramSpec(
		"http_request_duration_seconds",
		observabilityx.WithDescription("HTTP request duration in seconds."),
		observabilityx.WithUnit("s"),
		observabilityx.WithLabelKeys("method", "path", "status"),
	)
	httpAssetDeliveryTotalSpec = observabilityx.NewCounterSpec(
		"http_asset_delivery_total",
		observabilityx.WithDescription("Total number of asset delivery responses by delivery path."),
		observabilityx.WithLabelKeys("method", "path", "status", "delivery"),
	)
	httpAssetDeliveryDurationSpec = observabilityx.NewHistogramSpec(
		"http_asset_delivery_duration_seconds",
		observabilityx.WithDescription("Asset delivery request duration in seconds."),
		observabilityx.WithUnit("s"),
		observabilityx.WithLabelKeys("method", "path", "status", "delivery"),
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
		return func(c fiber.Ctx) error {
			runtimeMetrics.IncRequestsInFlight()
			defer runtimeMetrics.DecRequestsInFlight()

			if err := c.Next(); err != nil {
				return oops.In("server").Wrap(fmt.Errorf("run metrics middleware chain: %w", err))
			}
			return nil
		}
	}

	requestCounter := obs.Counter(httpRequestsTotalSpec)
	requestDuration := obs.Histogram(httpRequestDurationSpec)
	assetDeliveryCounter := obs.Counter(httpAssetDeliveryTotalSpec)
	assetDeliveryDuration := obs.Histogram(httpAssetDeliveryDurationSpec)

	return func(c fiber.Ctx) error {
		runtimeMetrics.IncRequestsInFlight()
		defer runtimeMetrics.DecRequestsInFlight()

		startedAt := time.Now()
		err := c.Next()
		duration := time.Since(startedAt).Seconds()

		requestAttrs := requestMetricsAttrs(c)
		requestCounter.Add(context.Background(), 1, requestAttrs...)
		requestDuration.Record(context.Background(), duration, requestAttrs...)

		deliveryAttrs := assetDeliveryMetricsAttrs(c)
		if len(deliveryAttrs) > 0 {
			assetDeliveryCounter.Add(context.Background(), 1, deliveryAttrs...)
			assetDeliveryDuration.Record(context.Background(), duration, deliveryAttrs...)
		}
		if err != nil {
			return oops.In("server").Wrap(fmt.Errorf("run metrics middleware chain: %w", err))
		}
		return nil
	}
}

func requestMetricsAttrs(c fiber.Ctx) []observabilityx.Attribute {
	return []observabilityx.Attribute{
		observabilityx.String("method", c.Method()),
		observabilityx.String("route", requestRoute(c)),
		observabilityx.String("status", strconv.Itoa(c.Response().StatusCode())),
	}
}

func assetDeliveryMetricsAttrs(c fiber.Ctx) []observabilityx.Attribute {
	delivery := getAssetDelivery(c)
	if delivery == "" {
		return nil
	}
	return append(requestMetricsAttrs(c), observabilityx.String("delivery", delivery))
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
