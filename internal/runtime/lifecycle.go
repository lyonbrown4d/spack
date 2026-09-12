package runtime

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

type mainHTTPListenerFactory func(context.Context, string, string) (net.Listener, error)

const defaultMainHTTPStartCleanupTimeout = 5 * time.Second

type mainHTTPRuntimeState struct {
	listen              mainHTTPListenerFactory
	beforeServe         func() error
	startCleanupTimeout time.Duration
	listener            net.Listener
	done                chan struct{}
	serveErr            error
}

type collectorRegistration struct {
	enabled   bool
	cfg       *config.Config
	providers []collectorProvider
}

func startMainHTTPRuntime(ctx context.Context, runtime mainHTTPRuntime) error {
	address := ":" + runtime.cfg.HTTP.GetPort()
	listener, err := runtime.state.listen(ctx, "tcp4", address)
	if err != nil {
		return oops.In("runtime").Owner("http runtime").Wrap(err)
	}

	done := make(chan struct{})
	runtime.state.listener = listener
	runtime.state.done = done
	if ctxErr := ctx.Err(); ctxErr != nil {
		closeErr := closeMainHTTPListener(runtime.state)
		close(done)
		return wrapMainHTTPRuntimeError(errors.Join(ctxErr, closeErr))
	}

	started := serveMainHTTPRuntime(runtime, listener, done)
	return waitForMainHTTPRuntimeStart(ctx, runtime, listener, started, done)
}

func serveMainHTTPRuntime(
	runtime mainHTTPRuntime,
	listener net.Listener,
	done chan<- struct{},
) <-chan struct{} {
	started := make(chan struct{})
	listenConfig := newMainHTTPListenConfig()
	listenConfig.BeforeServeFunc = func(*fiber.App) error {
		if runtime.state.beforeServe != nil {
			if err := runtime.state.beforeServe(); err != nil {
				return oops.In("runtime").Owner("http runtime").Wrap(err)
			}
		}
		close(started)
		return nil
	}

	go func() {
		err := runtime.app.Listener(listener, listenConfig)
		if errors.Is(err, net.ErrClosed) {
			err = nil
		}
		runtime.state.serveErr = err
		if err != nil {
			runtime.fatal.Report(err)
			runtime.logger.Error("HTTP runtime stopped", slog.Any("error", err))
		}
		close(done)
	}()
	return started
}

func waitForMainHTTPRuntimeStart(
	ctx context.Context,
	runtime mainHTTPRuntime,
	listener net.Listener,
	started <-chan struct{},
	done <-chan struct{},
) error {
	select {
	case <-started:
		logMainHTTPRuntimeStarted(runtime, listener)
		return nil
	case <-done:
		return mainHTTPRuntimeStoppedBeforeStart(runtime)
	case <-ctx.Done():
		closeErr := closeMainHTTPListener(runtime.state)
		waitErr := waitForMainHTTPRuntimeStartCleanup(ctx, runtime)
		return wrapMainHTTPRuntimeError(errors.Join(ctx.Err(), closeErr, waitErr))
	}
}

func waitForMainHTTPRuntimeStartCleanup(
	ctx context.Context,
	runtime mainHTTPRuntime,
) error {
	timeout := runtime.state.startCleanupTimeout
	if timeout <= 0 {
		timeout = defaultMainHTTPStartCleanupTimeout
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	return runtime.waitStopped(cleanupCtx)
}

func logMainHTTPRuntimeStarted(runtime mainHTTPRuntime, listener net.Listener) {
	runtime.logger.Info("HTTP runtime listening",
		slog.String("address", "http://"+listener.Addr().String()),
		slog.String("mount_path", runtime.cfg.Assets.Path),
		slog.Int("assets", runtime.cat.AssetCount()),
		slog.Int("variants", runtime.cat.VariantCount()),
	)
}

func mainHTTPRuntimeStoppedBeforeStart(runtime mainHTTPRuntime) error {
	closeErr := closeMainHTTPListener(runtime.state)
	if runtime.state.serveErr != nil {
		return wrapMainHTTPRuntimeError(errors.Join(runtime.state.serveErr, closeErr))
	}
	if closeErr != nil {
		return wrapMainHTTPRuntimeError(closeErr)
	}
	return wrapMainHTTPRuntimeError(errors.New("HTTP runtime stopped before serving"))
}

func wrapMainHTTPRuntimeError(err error) error {
	return oops.In("runtime").Owner("http runtime").Wrap(err)
}
func defaultMainHTTPListener(
	ctx context.Context,
	network string,
	address string,
) (net.Listener, error) {
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, network, address)
	if err != nil {
		return nil, oops.In("runtime").Owner("http listener").Wrap(err)
	}
	return listener, nil
}

func newMainHTTPListenConfig() fiber.ListenConfig {
	return fiber.ListenConfig{
		DisableStartupMessage: true,
		EnablePrefork:         false,
	}
}

func stopMainHTTPRuntime(ctx context.Context, runtime mainHTTPRuntime) error {
	runtime.logger.Info("Stop main HTTP runtime")

	shutdownErr := runtime.app.ShutdownWithContext(ctx)
	closeErr := closeMainHTTPListener(runtime.state)
	waitErr := runtime.waitStopped(ctx)
	if waitErr != nil &&
		runtime.state != nil &&
		runtime.state.serveErr != nil &&
		errors.Is(runtime.fatal.Err(), runtime.state.serveErr) &&
		errors.Is(waitErr, runtime.state.serveErr) {
		waitErr = nil
	}
	if err := errors.Join(shutdownErr, closeErr, waitErr); err != nil {
		return oops.In("runtime").Owner("http runtime").Wrap(err)
	}
	return nil
}

func closeMainHTTPListener(state *mainHTTPRuntimeState) error {
	if state == nil || state.listener == nil {
		return nil
	}
	if err := state.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return oops.In("runtime").Owner("http listener").Wrap(err)
	}
	return nil
}

func (r mainHTTPRuntime) waitStopped(ctx context.Context) error {
	if r.state == nil || r.state.done == nil {
		return nil
	}
	select {
	case <-r.state.done:
		if r.state.serveErr != nil {
			return oops.In("runtime").Owner("http runtime").Wrap(r.state.serveErr)
		}
		return nil
	case <-ctx.Done():
		return oops.In("runtime").Owner("http runtime").Wrap(ctx.Err())
	}
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
		logger.Error("Runtime collector registration failed", slog.Any("error", err))
		return nil
	}
	logger.Info("Runtime collectors registered",
		slog.String("metrics", registration.cfg.Metrics.Prefix),
	)
	return nil
}
