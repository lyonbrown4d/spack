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
	"github.com/samber/lo"
	"github.com/samber/oops"
)

type collectorRegistration struct {
	enabled   bool
	cfg       *config.Config
	providers []collectorProvider
}

func startMainHTTPRuntime(_ context.Context, runtime mainHTTPRuntime) error {
	go func() {
		address := "127.0.0.1:" + runtime.cfg.HTTP.GetPort()
		listenConfig := newMainHTTPListenConfig()
		runtime.logger.Info("HTTP runtime listening",
			slog.String("address", "http://"+address),
			slog.String("mount_path", runtime.cfg.Assets.Path),
			slog.Int("assets", runtime.cat.AssetCount()),
			slog.Int("variants", runtime.cat.VariantCount()),
		)
		if err := runtime.app.Listen(":"+runtime.cfg.HTTP.GetPort(), listenConfig); err != nil {
			runtime.logger.Error("HTTP runtime stopped", slog.String("err", err.Error()))
		}
	}()
	return nil
}

func newMainHTTPListenConfig() fiber.ListenConfig {
	return fiber.ListenConfig{
		DisableStartupMessage: true,
		EnablePrefork:         false,
	}
}

func stopMainHTTPRuntime(ctx context.Context, runtime mainHTTPRuntime) error {
	runtime.logger.Info("Stop main HTTP runtime")
	if err := runtime.app.ShutdownWithContext(ctx); err != nil {
		return oops.In("runtime").Owner("http runtime").Wrap(err)
	}
	return nil
}

func buildCollectorRegistration(
	cfg *config.Config,
	providers ...collectorProvider,
) *collectorRegistration {
	if !cfg.Metrics.Enable {
		return &collectorRegistration{}
	}
	return &collectorRegistration{
		enabled:   true,
		cfg:       cfg,
		providers: providers,
	}
}

type collectorProvider interface {
	Collectors() []prometheus.Collector
}

func registerRuntimeCollectors(cfg *config.Config, providers []collectorProvider) error {
	providers = lo.Concat(providers, []collectorProvider{
		metrics.NewBuildInfoMetrics("spack"),
		metrics.NewRuntimeInfoMetrics("spack", cfg, time.Now().UTC()),
	})
	for _, provider := range providers {
		if err := registerCollectorProvider(provider); err != nil {
			return err
		}
	}
	return nil
}

func registerCollectorProvider(provider collectorProvider) error {
	if provider == nil {
		return nil
	}
	collectors := provider.Collectors()
	if len(collectors) == 0 {
		return nil
	}
	for _, collector := range collectors {
		if err := prometheus.Register(collector); err != nil {
			if alreadyRegistered, ok := errors.AsType[prometheus.AlreadyRegisteredError](err); ok {
				_ = alreadyRegistered.ExistingCollector
				continue
			}
			return oops.In("runtime").Owner("runtime collectors").Wrap(err)
		}
	}
	return nil
}

func startRuntimeCollectors(_ context.Context, logger *slog.Logger, registration *collectorRegistration) error {
	if registration == nil || !registration.enabled {
		return nil
	}
	if err := registerRuntimeCollectors(registration.cfg, registration.providers); err != nil {
		logger.Error("Runtime collector registration failed", slog.String("err", err.Error()))
		return nil
	}
	logger.Info("Runtime collectors registered",
		slog.String("metrics", registration.cfg.Metrics.Prefix),
	)
	return nil
}
