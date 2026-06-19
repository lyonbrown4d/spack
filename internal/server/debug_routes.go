package server

import (
	"context"
	"log/slog"
	"strings"

	"github.com/arcgolabs/observabilityx/prometheus"
	"github.com/arl/statsviz"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
	"github.com/gofiber/fiber/v3/middleware/pprof"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/samber/oops"
)

const statsvizRoot = "/debug/statsviz"

type diagnosticsRoutesRuntime struct {
	cfg            *config.Config
	logger         *slog.Logger
	metricsAdapter *prometheus.Adapter
	cat            catalog.Catalog
	statsviz       *statsviz.Server
}

func newDiagnosticsRoutesRuntime(
	cfg *config.Config,
	logger *slog.Logger,
	metricsAdapter *prometheus.Adapter,
	cat catalog.Catalog,
) *diagnosticsRoutesRuntime {
	runtime := &diagnosticsRoutesRuntime{
		cfg:            cfg,
		logger:         logger,
		metricsAdapter: metricsAdapter,
		cat:            cat,
	}
	if cfg == nil || !cfg.Debug.Enable {
		return runtime
	}

	statsvizServer, err := statsviz.NewServer(statsviz.Root(statsvizRoot))
	if err != nil {
		if logger != nil {
			logger.Error("Statsviz route unavailable", slog.String("err", err.Error()))
		}
		return runtime
	}
	runtime.statsviz = statsvizServer
	return runtime
}

func registerDiagnosticsRoutes(app *fiber.App, runtime *diagnosticsRoutesRuntime) {
	if app == nil || runtime == nil || runtime.cfg == nil {
		return
	}

	if runtime.cfg.Metrics.Enable {
		registerPrometheusRoute(app, runtime)
	}
	if runtime.cfg.Debug.Enable {
		registerCatalogRoute(app, runtime.cat)
		registerPprofRoutes(app, runtime.cfg)
		registerStatsvizRoutes(app, runtime)
	}
}

func registerPrometheusRoute(app *fiber.App, runtime *diagnosticsRoutesRuntime) {
	if runtime.metricsAdapter == nil {
		if runtime.logger != nil {
			runtime.logger.Warn("Prometheus route unavailable", slog.String("err", "metrics adapter is not configured"))
		}
		return
	}
	app.Get(runtime.cfg.Metrics.Prefix, adaptor.HTTPHandler(runtime.metricsAdapter.Handler()))
}

func registerCatalogRoute(app *fiber.App, cat catalog.Catalog) {
	app.Get("/catalog", func(c fiber.Ctx) error {
		if cat == nil {
			return fiber.ErrServiceUnavailable
		}
		return c.JSON(cat.Snapshot())
	})
}

func registerPprofRoutes(app *fiber.App, cfg *config.Config) {
	app.Use(pprof.New(pprof.Config{
		Prefix: normalizedPprofPrefix(cfg.Debug.PprofPrefix),
	}))
}

func registerStatsvizRoutes(app *fiber.App, runtime *diagnosticsRoutesRuntime) {
	if runtime.statsviz == nil {
		return
	}

	app.Get(statsvizRoot, func(c fiber.Ctx) error {
		c.Set(fiber.HeaderLocation, statsvizRoot+"/")
		return c.SendStatus(fiber.StatusTemporaryRedirect)
	})
	app.Get(statsvizRoot+"/ws", adaptor.HTTPHandlerFunc(runtime.statsviz.Ws()))
	app.Get(statsvizRoot+"/*", adaptor.HTTPHandlerFunc(runtime.statsviz.Index()))
}

func normalizedPprofPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "/" {
		return ""
	}
	return strings.TrimRight(prefix, "/")
}

func stopDiagnosticsRoutesRuntime(_ context.Context, runtime *diagnosticsRoutesRuntime) error {
	if runtime == nil || runtime.statsviz == nil {
		return nil
	}
	if err := runtime.statsviz.Close(); err != nil {
		return oops.In("server").Owner("statsviz").Wrap(err)
	}
	return nil
}
