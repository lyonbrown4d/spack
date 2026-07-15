package server

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/eventx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	appEvent "github.com/lyonbrown4d/spack/internal/event"
	"github.com/samber/mo"
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
	unsubscribes         []func()
}

type preparedSubscription struct {
	name      string
	subscribe func() (func(), error)
}

func newPreparedService(
	cfg *config.Config,
	cat catalog.Catalog,
	logger *slog.Logger,
	resourceHints *resourceHintService,
	bus eventx.BusRuntime,
	metrics *RuntimeMetrics,
) *PreparedService {
	return &PreparedService{
		cfg:           cfg,
		cat:           cat,
		logger:        logger,
		resourceHints: resourceHints,
		bus:           bus,
		metrics:       metrics,
	}
}

func (s *PreparedService) Rebuild(ctx context.Context) error {
	if s == nil {
		return nil
	}

	s.rebuildMu.Lock()
	defer s.rebuildMu.Unlock()

	startedAt := time.Now()
	compiler := newPreparedCompiler(s.cfg, s.resourceHints, s.logger)
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
			slog.String("err", err.Error()),
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

func (s *PreparedService) start(_ context.Context) error {
	if s == nil || s.bus == nil || len(s.unsubscribes) > 0 {
		return nil
	}

	unsubscribes := cxlist.NewListWithCapacity[func()](3)
	for _, subscription := range s.subscriptions().Values() {
		unsubscribe, err := subscription.subscribe()
		if err != nil {
			unsubscribes.Each(func(_ int, existing func()) {
				if existing != nil {
					existing()
				}
			})
			return fmt.Errorf("subscribe prepared %s: %w", subscription.name, err)
		}
		unsubscribes.Add(unsubscribe)
	}

	s.unsubscribes = unsubscribes.Values()
	return nil
}

func (s *PreparedService) stop(_ context.Context) error {
	if s == nil {
		return nil
	}
	cxlist.NewList[func()](s.unsubscribes...).Each(func(_ int, unsubscribe func()) {
		if unsubscribe != nil {
			unsubscribe()
		}
	})
	s.unsubscribes = nil
	return nil
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
	if s == nil {
		return
	}
	s.rebuildAgain.Store(true)
	if !s.rebuildWorkerRunning.CompareAndSwap(false, true) {
		return
	}
	go s.runRebuildWorker(ctx, reason)
}

func (s *PreparedService) runRebuildWorker(ctx context.Context, reason string) {
	for {
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
			slog.String("err", err.Error()),
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
