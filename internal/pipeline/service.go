package pipeline

import (
	"context"
	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	cxset "github.com/arcgolabs/collectionx/set"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/cachepolicy"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/panjf2000/ants/v2"
	"github.com/samber/oops"
	"golang.org/x/sync/singleflight"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Service struct {
	cfg        *config.Compression
	logger     *slog.Logger
	catalog    catalog.Catalog
	metrics    *Metrics
	obs        observabilityx.Observability
	catMetrics *catalog.RuntimeMetrics
	stages     *cxlist.List[Stage]
	bus        eventx.BusRuntime

	tasks          chan Request
	lazyWorkerPool *ants.PoolWithFunc
	workerStopOnce sync.Once
	workerCancel   context.CancelFunc
	workerClosing  atomic.Bool
	enqueueMu      sync.RWMutex
	sf             singleflight.Group
	pending        *cxset.ConcurrentSet[string]

	cleanupMu sync.Mutex
	cleanup   *cleanupRuntime

	variantHits *cxmapping.ConcurrentMap[string, time.Time]
	warmWorkers *asyncx.Settings

	artifactPolicy cachepolicy.ArtifactPolicy

	variantServedUnsubscribe func()
}

func (s *Service) Enqueue(request Request) {
	if !s.cfg.PipelineEnabled() || s.cfg.NormalizedMode() != config.CompressionModeLazy {
		return
	}
	if strings.TrimSpace(request.AssetPath) == "" || s.workerClosing.Load() {
		return
	}

	key := requestKey(request)
	if !s.pending.AddIfAbsent(key) {
		if s.metrics != nil {
			s.metrics.EnqueueDeduplicatedTotal.Inc()
		}
		return
	}

	s.enqueueMu.RLock()
	defer s.enqueueMu.RUnlock()
	if s.workerClosing.Load() {
		s.pending.Remove(key)
		s.recordQueueDrop(request)
		return
	}

	select {
	case s.tasks <- request:
		s.updateQueueLengthMetric()
	default:
		s.pending.Remove(key)
		s.recordQueueDrop(request)
	}
}

func (s *Service) recordQueueDrop(request Request) {
	if s.metrics != nil {
		s.metrics.EnqueueDroppedTotal.Inc()
	}
	s.logger.Debug("Pipeline queue full",
		slog.String("asset", request.AssetPath),
		slog.Int("queue_len", len(s.tasks)),
		slog.Int("queue_cap", cap(s.tasks)),
	)
}

func (s *Service) MarkVariantHit(path string) {
	s.markVariantHitAt(path, time.Now())
}

func (s *Service) markVariantHitAt(path string, hitAt time.Time) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	s.variantHits.Set(path, hitAt)
}

func (s *Service) Warm(ctx context.Context) error {
	if !s.cfg.PipelineEnabled() || s.cfg.NormalizedMode() != config.CompressionModeWarmup {
		return nil
	}

	err := asyncx.RunList[*catalog.Asset](ctx, s.obs, s.warmWorkers, "pipeline_warm", s.catalog.AllAssets(), func(ctx context.Context, asset *catalog.Asset) error {
		s.process(ctx, Request{AssetPath: asset.Path})
		return nil
	})
	if err != nil {
		return oops.Wrapf(err, "warm pipeline")
	}
	return nil
}

func (s *Service) process(ctx context.Context, request Request) {
	asset, ok := s.catalog.FindAsset(request.AssetPath)
	if !ok {
		return
	}
	if ctx.Err() != nil {
		return
	}

	s.stages.Range(func(_ int, stage Stage) bool {
		stage.Plan(asset, request).Range(func(_ int, task Task) bool {
			s.executeStageTaskBatch(ctx, stage, asset, task).Range(func(_ int, variant *catalog.Variant) bool {
				s.upsertStageVariant(ctx, stage, asset, variant)
				return true
			})
			return true
		})
		return true
	})
}

func (s *Service) finishRequest(key string) {
	s.pending.Remove(key)
}
