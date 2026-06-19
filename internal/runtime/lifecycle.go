package runtime

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/oops"
)

type debugRuntime struct {
	enabled bool
	cfg     *config.Config
	deps    debugRuntimeDeps
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
	return &debugRuntime{
		enabled: true,
		cfg:     cfg,
		deps:    deps,
	}
}

type debugCollectorProvider interface {
	Collectors() []prometheus.Collector
}

func registerDebugRuntimeCollectors(cfg *config.Config, deps debugRuntimeDeps) error {
	providers := []debugCollectorProvider{
		deps.pipelineMetrics,
		deps.catMetrics,
		deps.serverMetrics,
		deps.taskMetrics,
		deps.asyncMetrics,
		metrics.NewBuildInfoMetrics("spack"),
		metrics.NewRuntimeInfoMetrics("spack", cfg, time.Now().UTC()),
	}
	for _, provider := range providers {
		if err := registerDebugCollectors(provider); err != nil {
			return err
		}
	}
	return nil
}

func registerDebugCollectors(provider debugCollectorProvider) error {
	if provider == nil {
		return nil
	}
	collectors := provider.Collectors()
	if len(collectors) == 0 {
		return nil
	}
	for _, collector := range collectors {
		if err := prometheus.Register(collector); err != nil {
			var alreadyRegistered prometheus.AlreadyRegisteredError
			if errors.As(err, &alreadyRegistered) {
				continue
			}
			return oops.In("runtime").Owner("debug collectors").Wrap(err)
		}
	}
	return nil
}

func startDebugRuntime(_ context.Context, logger *slog.Logger, runtime *debugRuntime) error {
	if runtime == nil || !runtime.enabled {
		return nil
	}
	if err := registerDebugRuntimeCollectors(runtime.cfg, runtime.deps); err != nil {
		logger.Error("Debug runtime collector registration failed", slog.String("err", err.Error()))
		return nil
	}
	logger.Info("Debug runtime collectors registered",
		slog.String("metrics", runtime.cfg.Metrics.Prefix),
		slog.String("pprof", "/debug/pprof"),
		slog.String("statsviz", "/debug/statsviz"),
	)
	return nil
}

func stopDebugRuntime(ctx context.Context, runtime *debugRuntime) error {
	return nil
}
