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
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/samber/oops"
)

const statsvizRoot = "/debug/statsviz"

type debugRoutesRuntime struct {
	cfg            *config.Config
	logger         *slog.Logger
	metricsAdapter *prometheus.Adapter
	statsviz       *statsviz.Server
}

func newDebugRoutesRuntime(
	cfg *config.Config,
	logger *slog.Logger,
	metricsAdapter *prometheus.Adapter,
) *debugRoutesRuntime {
	runtime := &debugRoutesRuntime{
		cfg:            cfg,
		logger:         logger,
		metricsAdapter: metricsAdapter,
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

func registerDebugRoutes(app *fiber.App, runtime *debugRoutesRuntime) {
	if app == nil || runtime == nil || runtime.cfg == nil || !runtime.cfg.Debug.Enable {
		return
	}

	registerPrometheusRoute(app, runtime)
	registerPprofRoutes(app, runtime.cfg)
	registerStatsvizRoutes(app, runtime)
}

func registerPrometheusRoute(app *fiber.App, runtime *debugRoutesRuntime) {
	if runtime.metricsAdapter == nil {
		if runtime.logger != nil {
			runtime.logger.Warn("Prometheus route unavailable", slog.String("err", "metrics adapter is not configured"))
		}
		return
	}
	app.Get(runtime.cfg.Metrics.Prefix, adaptor.HTTPHandler(runtime.metricsAdapter.Handler()))
}

func registerPprofRoutes(app *fiber.App, cfg *config.Config) {
	app.Use(pprof.New(pprof.Config{
		Prefix: normalizedPprofPrefix(cfg.Debug.PprofPrefix),
	}))
}

func registerStatsvizRoutes(app *fiber.App, runtime *debugRoutesRuntime) {
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

func stopDebugRoutesRuntime(_ context.Context, runtime *debugRoutesRuntime) error {
	if runtime == nil || runtime.statsviz == nil {
		return nil
	}
	if err := runtime.statsviz.Close(); err != nil {
		return oops.In("server").Owner("statsviz").Wrap(err)
	}
	return nil
}
