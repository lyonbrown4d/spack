package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/arl/statsviz"
	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/mo"
	"github.com/samber/oops"
)

type debugRuntime struct {
	enabled    bool
	server     *http.Server
	liveURL    string
	metricsURL string
	statsviz   string
}

func startHTTPRuntime(_ context.Context, runtime httpRuntime) error {
	go func() {
		address := "127.0.0.1:" + runtime.cfg.HTTP.GetPort()
		listenConfig := newHTTPListenConfig(runtime.cfg)
		runtime.logger.Info("HTTP runtime listening",
			slog.String("address", "http://"+address),
			slog.String("mount_path", runtime.cfg.Assets.Path),
			slog.Int("assets", runtime.cat.AssetCount()),
			slog.Int("variants", runtime.cat.VariantCount()),
			slog.Bool("prefork", listenConfig.EnablePrefork),
		)
		if err := runtime.app.Listen(":"+runtime.cfg.HTTP.GetPort(), listenConfig); err != nil {
			runtime.logger.Error("HTTP runtime stopped", slog.String("err", err.Error()))
		}
	}()
	return nil
}

func newHTTPListenConfig(cfg *config.Config) fiber.ListenConfig {
	return fiber.ListenConfig{
		DisableStartupMessage: true,
		EnablePrefork:         cfg.HTTP.Prefork,
	}
}

func stopHTTPRuntime(ctx context.Context, runtime httpRuntime) error {
	runtime.logger.Info("stop http runtime")
	if err := runtime.app.ShutdownWithContext(ctx); err != nil {
		return oops.In("runtime").Owner("http runtime").Wrap(err)
	}
	return nil
}

func buildDebugRuntime(
	cfg *config.Config,
	logger *slog.Logger,
	deps debugRuntimeDeps,
) *debugRuntime {
	if !cfg.Debug.Enable {
		return &debugRuntime{}
	}

	mux := http.NewServeMux()
	registerDebugCollectors(deps.pipelineMetrics)
	registerDebugCollectors(deps.catMetrics)
	registerDebugCollectors(deps.serverMetrics)
	registerDebugCollectors(deps.taskMetrics)
	registerDebugCollectors(deps.asyncMetrics)
	registerDebugCollectors(metrics.NewBuildInfoMetrics("spack"))
	registerDebugCollectors(metrics.NewRuntimeInfoMetrics("spack", cfg, time.Now().UTC()))
	if deps.metricsAdapter == nil {
		logger.Error("Debug runtime registration failed", slog.String("err", "metrics adapter is not configured"))
		return &debugRuntime{}
	}
	mux.Handle(cfg.Metrics.Prefix, deps.metricsAdapter.Handler())
	if err := statsviz.Register(mux); err != nil {
		logger.Error("Debug runtime registration failed", slog.String("err", err.Error()))
		return &debugRuntime{}
	}

	address := fmt.Sprintf(cfg.Debug.Address+":%d", cfg.Debug.LivePort)
	return &debugRuntime{
		enabled: true,
		server: &http.Server{
			Addr:              address,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
		liveURL:    fmt.Sprintf("http://127.0.0.1:%d", cfg.Debug.LivePort),
		metricsURL: fmt.Sprintf("http://127.0.0.1:%d%s", cfg.Debug.LivePort, cfg.Metrics.Prefix),
		statsviz:   fmt.Sprintf("http://127.0.0.1:%d/debug/statsviz", cfg.Debug.LivePort),
	}
}

type debugCollectorProvider interface {
	Collectors() []prometheus.Collector
}

func registerDebugCollectors(provider debugCollectorProvider) {
	if provider == nil {
		return
	}
	collectors := provider.Collectors()
	if len(collectors) == 0 {
		return
	}
	for _, collector := range collectors {
		if err := prometheus.Register(collector); err != nil {
			var alreadyRegistered prometheus.AlreadyRegisteredError
			if errors.As(err, &alreadyRegistered) {
				continue
			}
			panic(err)
		}
	}
}

func startDebugRuntime(_ context.Context, logger *slog.Logger, runtime *debugRuntime) error {
	server, ok := debugServer(runtime).Get()
	if !ok {
		return nil
	}

	go func() {
		logger.Info("Debug runtime listening",
			slog.String("address", runtime.liveURL),
			slog.String("metrics", runtime.metricsURL),
			slog.String("statsviz", runtime.statsviz),
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Debug runtime stopped", slog.String("err", err.Error()))
		}
	}()
	return nil
}

func stopDebugRuntime(ctx context.Context, runtime *debugRuntime) error {
	server, ok := debugServer(runtime).Get()
	if !ok {
		return nil
	}
	if err := server.Shutdown(ctx); err != nil {
		return oops.In("runtime").Owner("debug runtime").Wrap(err)
	}
	return nil
}

func debugServer(runtime *debugRuntime) mo.Option[*http.Server] {
	if runtime == nil || !runtime.enabled || runtime.server == nil {
		return mo.None[*http.Server]()
	}
	return mo.Some(runtime.server)
}
