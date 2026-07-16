package server

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/eventx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	appEvent "github.com/lyonbrown4d/spack/internal/event"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/samber/mo"
	"github.com/samber/oops"
)

type PreparedService struct {
	cfg                  *config.Config
	cat                  catalog.Catalog
	logger               *slog.Logger
	resourceHints        *resourceHintService
	bus                  eventx.BusRuntime
	metrics              *RuntimeMetrics
	snapshot             atomic.Pointer[preparedSnapshot]
	rebuildMu            sync.Mutex
	rebuildWorkerRunning atomic.Bool
	rebuildAgain         atomic.Bool
	rebuildStopped       atomic.Bool
	rebuildWG            sync.WaitGroup
	lifecycleCtx         context.Context
	lifecycleCancel      context.CancelFunc
	unsubscribes         []func()
	fileSources          *serverFileSources
}

type preparedSubscription struct {
	name      string
	subscribe func() (func(), error)
}

func newPreparedService(
	cfg *config.Config,
	cat catalog.Catalog,
	logger *slog.Logger,
	bus eventx.BusRuntime,
	metrics *RuntimeMetrics,
	src *source.LocalFS,
) *PreparedService {
	return &PreparedService{
		cfg:           cfg,
		cat:           cat,
		logger:        logger,
		resourceHints: newResourceHintService(cfg, logger, src),
		bus:           bus,
		metrics:       metrics,
		fileSources:   newServerFileSources(cfg, src, cat, logger),
	}
}

func (s *PreparedService) Rebuild(ctx context.Context) error {
	if s == nil {
		return nil
	}

	s.rebuildMu.Lock()
	defer s.rebuildMu.Unlock()

	startedAt := time.Now()
	s.fileSources = mergeServerFileSources(s.fileSources, newServerFileSources(s.cfg, nil, s.cat, s.logger))
	compiler := newPreparedCompiler(s.cfg, s.resourceHints, s.logger, s.fileSources)
	snapshot, err := compiler.Compile(ctx, s.cat)
	if err != nil {
		return preparedCompileError(err)
	}
	s.snapshot.Store(snapshot)
	duration := time.Since(startedAt)
	s.metrics.RecordPreparedSnapshot(duration, snapshot.assets, snapshot.assets+snapshot.variants, snapshot.bodyEntries, snapshot.bodyBytes)
	if s.logger != nil {
		s.logger.Info("Prepared snapshot ready",
			slog.Int("assets", snapshot.assets),
			slog.Int("variants", snapshot.variants),
			slog.Int("body_entries", snapshot.bodyEntries),
			slog.Int64("body_bytes", snapshot.bodyBytes),
			slog.Duration("duration", duration),
		)
	}
	return nil
}

func (s *PreparedService) Resolve(request preparedRequest) mo.Option[preparedSelection] {
	snapshot := s.current()
	if snapshot == nil {
		return mo.None[preparedSelection]()
	}
	return snapshot.resolve(s.cfg.Assets, request)
}

func (s *PreparedService) DeleteVariantArtifact(ctx context.Context, artifactPath string) bool {
	if s == nil || s.cat == nil {
		return false
	}
	if !s.cat.DeleteVariantByArtifactPath(artifactPath) {
		return false
	}
	if err := s.Rebuild(ctx); err != nil && s.logger != nil {
		s.logger.Error("Prepared snapshot rebuild failed after variant delete",
			slog.String("artifact_path", artifactPath),
			slog.Any("error", err),
		)
	}
	return true
}

func (s *PreparedService) current() *preparedSnapshot {
	if s == nil {
		return nil
	}
	return s.snapshot.Load()
}

func (s *PreparedService) start(ctx context.Context) error {
	if s == nil || s.bus == nil || len(s.unsubscribes) > 0 {
		return nil
	}

	s.lifecycleCtx, s.lifecycleCancel = context.WithCancel(context.WithoutCancel(ctx))
	s.rebuildStopped.Store(false)

	unsubscribes := cxlist.NewListWithCapacity[func()](3)
	for _, subscription := range s.subscriptions().Values() {
		unsubscribe, err := subscription.subscribe()
		if err != nil {
			unsubscribes.Each(func(_ int, existing func()) {
				if existing != nil {
					existing()
				}
			})
			s.cancelLifecycle()
			return oops.Wrapf(err, "subscribe prepared %s", subscription.name)
		}
		unsubscribes.Add(unsubscribe)
	}

	s.unsubscribes = unsubscribes.Values()
	return nil
}

func (s *PreparedService) stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.rebuildStopped.Store(true)
	cxlist.NewList[func()](s.unsubscribes...).Each(func(_ int, unsubscribe func()) {
		if unsubscribe != nil {
			unsubscribe()
		}
	})
	s.unsubscribes = nil
	s.cancelLifecycle()
	if err := s.waitRebuildWorkers(ctx); err != nil {
		return err
	}
	s.lifecycleCtx = nil
	s.lifecycleCancel = nil
	return nil
}

func (s *PreparedService) cancelLifecycle() {
	if s.lifecycleCancel != nil {
		s.lifecycleCancel()
	}
}

func (s *PreparedService) waitRebuildWorkers(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.rebuildWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return oops.Wrapf(ctx.Err(), "wait for prepared rebuild worker")
	}
}

func (s *PreparedService) subscriptions() *cxlist.List[preparedSubscription] {
	return cxlist.NewList[preparedSubscription](
		preparedSubscription{
			name: "variant generated",
			subscribe: func() (func(), error) {
				return eventx.Subscribe(s.bus, func(ctx context.Context, _ appEvent.VariantGenerated) error {
					s.rebuildAsync(ctx, "variant generated")
					return nil
				})
			},
		},
		preparedSubscription{
			name: "variant removed",
			subscribe: func() (func(), error) {
				return eventx.Subscribe(s.bus, func(ctx context.Context, _ appEvent.VariantRemoved) error {
					s.rebuildAsync(ctx, "variant removed")
					return nil
				})
			},
		},
		preparedSubscription{
			name: "catalog changed",
			subscribe: func() (func(), error) {
				return eventx.Subscribe(s.bus, func(ctx context.Context, _ appEvent.CatalogChanged) error {
					s.rebuildAsync(ctx, "catalog changed")
					return nil
				})
			},
		},
	)
}

func (s *PreparedService) rebuildAsync(ctx context.Context, reason string) {
	if s == nil || s.rebuildStopped.Load() {
		return
	}
	s.rebuildAgain.Store(true)
	if !s.rebuildWorkerRunning.CompareAndSwap(false, true) {
		return
	}
	workerCtx := s.rebuildContext(ctx)
	s.rebuildWG.Go(func() {
		s.runRebuildWorker(workerCtx, reason)
	})
}

func (s *PreparedService) rebuildContext(ctx context.Context) context.Context {
	if s.lifecycleCtx != nil {
		return s.lifecycleCtx
	}
	if ctx == nil {
		return context.TODO()
	}
	return context.WithoutCancel(ctx)
}

func (s *PreparedService) runRebuildWorker(ctx context.Context, reason string) {
	for {
		if s.rebuildStopped.Load() || ctx.Err() != nil {
			s.rebuildAgain.Store(false)
			s.rebuildWorkerRunning.Store(false)
			return
		}
		if s.rebuildAgain.Swap(false) {
			s.rebuildOnce(ctx, reason)
			continue
		}
		if !s.releaseRebuildWorker() {
			return
		}
	}
}

func (s *PreparedService) rebuildOnce(ctx context.Context, reason string) {
	if err := s.Rebuild(ctx); err != nil && s.logger != nil {
		s.logger.Error("Prepared snapshot rebuild failed",
			slog.String("reason", reason),
			slog.Any("error", err),
		)
	}
}

func (s *PreparedService) releaseRebuildWorker() bool {
	s.rebuildWorkerRunning.Store(false)
	if !s.rebuildAgain.Load() {
		return false
	}
	return s.rebuildWorkerRunning.CompareAndSwap(false, true)
}
