package pipeline

import (
	"context"
	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	cxset "github.com/arcgolabs/collectionx/set"
	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/cachepolicy"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/samber/oops"
	"log/slog"
	"os"
	"strings"
	"time"
)

var (
	pipelineStageRunsTotalSpec = observabilityx.NewCounterSpec(
		"pipeline_stage_runs_total",
		observabilityx.WithDescription("Total number of pipeline stage executions."),
		observabilityx.WithLabelKeys("stage", "result"),
	)
	pipelineStageDurationSpec = observabilityx.NewHistogramSpec(
		"pipeline_stage_duration_seconds",
		observabilityx.WithDescription("Pipeline stage execution duration in seconds."),
		observabilityx.WithUnit("s"),
		observabilityx.WithLabelKeys("stage", "result"),
	)
	pipelineVariantsGeneratedTotalSpec = observabilityx.NewCounterSpec(
		"pipeline_variants_generated_total",
		observabilityx.WithDescription("Total number of generated variants produced by pipeline stages."),
		observabilityx.WithLabelKeys("stage"),
	)
	pipelineVariantsGeneratedBytesTotalSpec = observabilityx.NewCounterSpec(
		"pipeline_variants_generated_bytes_total",
		observabilityx.WithDescription("Total size in bytes of generated variants produced by pipeline stages."),
		observabilityx.WithUnit("By"),
		observabilityx.WithLabelKeys("stage"),
	)
)

func resolveQueueSize(cfg *config.Compression, workers int) int {
	if cfg.NormalizedMode() != config.CompressionModeLazy {
		return 0
	}
	queueSize := cfg.QueueCapacity()
	if queueSize < 1 {
		return workers * 64
	}
	return queueSize
}

func newServiceState(
	cfg *config.Compression,
	logger *slog.Logger,
	cat catalog.Catalog,
	services serviceDeps,
	queueSize int,
) *Service {
	stages := services.stages
	if stages == nil {
		stages = cxlist.NewList[Stage]()
	}
	return &Service{
		cfg:            cfg,
		logger:         logger,
		catalog:        cat,
		metrics:        services.metrics,
		obs:            observabilityx.Normalize(services.obs, logger),
		catMetrics:     services.catMetrics,
		stages:         stages,
		bus:            services.bus,
		tasks:          make(chan Request, queueSize),
		pending:        cxset.NewConcurrentSetWithCapacity[string](queueSize),
		variantHits:    cxmapping.NewConcurrentMapWithCapacity[string, time.Time](queueSize),
		warmWorkers:    services.workers,
		artifactPolicy: cachepolicy.NewArtifactPolicy(cfg),
	}
}
func (s *Service) initializeMetrics(queueSize int) {
	if s.metrics == nil {
		return
	}
	s.metrics.QueueCapacity.Set(float64(queueSize))
	s.metrics.QueueLength.Set(0)
}
func (s *Service) start(ctx context.Context, workers, queueSize int) error {
	if !s.cfg.PipelineEnabled() {
		s.logger.Info("Pipeline disabled")
		return nil
	}
	if err := s.subscribeVariantServed(); err != nil {
		return oops.Wrapf(err, "subscribe variant served events")
	}
	if strings.TrimSpace(s.cfg.CacheDir) == "" {
		return nil
	}
	if err := os.MkdirAll(s.cfg.CacheDir, 0o750); err != nil {
		return oops.Wrapf(err, "create pipeline cache directory")
	}
	if s.cfg.NormalizedMode() == config.CompressionModeLazy {
		if err := s.startWorkers(ctx, workers); err != nil {
			return err
		}
		s.logLazyWorkersStarted(workers, queueSize)
	} else {
		s.logCompilerWarmupConfigured()
	}
	s.startCleanupIfNeeded(ctx)
	return nil
}

func (s *Service) logLazyWorkersStarted(workers, queueSize int) {
	s.logger.Info("Legacy lazy pipeline workers started",
		slog.Int("workers", workers),
		slog.Int("legacy_queue_capacity", queueSize),
		slog.String("mode", s.cfg.NormalizedMode()),
		slog.String("cache_dir", s.cfg.CacheDir),
	)
}

func (s *Service) logCompilerWarmupConfigured() {
	s.logger.Info("Pipeline generation configured for compiler warmup",
		slog.String("mode", s.cfg.NormalizedMode()),
		slog.String("cache_dir", s.cfg.CacheDir),
	)
}

func (s *Service) startCleanupIfNeeded(ctx context.Context) {
	if !s.cleanupEnabled() {
		return
	}

	interval := s.cfg.ParsedCleanupInterval()
	s.cleanup = newCleanupRuntime(interval, s.cleanupOnce)
	s.cleanup.Start(ctx)
	s.logger.Info("Pipeline cleanup enabled",
		slog.String("interval", interval.String()),
		slog.String("max_age", s.artifactPolicy.DefaultMaxAge().String()),
		slog.String("encoding_max_age", s.artifactPolicy.MaxAge("encoding").String()),
		slog.String("image_max_age", s.artifactPolicy.MaxAge("image").String()),
		slog.Int64("max_cache_bytes", s.artifactPolicy.MaxCacheBytes()),
		slog.Int64("encoding_max_cache_bytes", s.artifactPolicy.MaxCacheBytesForNamespace("encoding")),
		slog.Int64("image_max_cache_bytes", s.artifactPolicy.MaxCacheBytesForNamespace("image")),
	)
}

func (s *Service) cleanupEnabled() bool {
	return s.artifactPolicy != nil && s.artifactPolicy.Enabled()
}

func (s *Service) stop(ctx context.Context) error {
	s.unsubscribeVariantServed()
	if err := s.stopCleanup(ctx); err != nil {
		return err
	}
	return s.stopWorkers(ctx)
}

func (s *Service) stopCleanup(ctx context.Context) error {
	if s.cleanup == nil {
		return nil
	}
	if err := s.cleanup.Stop(ctx); err != nil {
		return err
	}
	s.cleanup = nil
	return nil
}

func (s *Service) executeStageTask(ctx context.Context, stage Stage, asset *catalog.Asset, task Task) *catalog.Variant {
	variants := s.executeStageTaskBatch(ctx, stage, asset, task)
	variant, _ := variants.Get(0)
	return variant
}

func (s *Service) executeStageTaskBatch(ctx context.Context, stage Stage, asset *catalog.Asset, task Task) *cxlist.List[*catalog.Variant] {
	startedAt := time.Now()
	key := buildStageTaskKey(stage, asset, task)
	variantValue, err, _ := s.sf.Do(key, func() (any, error) {
		return executeStageTaskValue(ctx, stage, asset, task)
	})
	if err != nil {
		s.recordStageTaskError(ctx, stage, asset, startedAt, err)
		return cxlist.NewList[*catalog.Variant]()
	}

	variants := stageTaskVariants(variantValue)
	if variants.IsEmpty() {
		s.recordStageRunMetrics(ctx, stage.Name(), "empty", startedAt)
		return cxlist.NewList[*catalog.Variant]()
	}
	s.recordStageRunMetrics(ctx, stage.Name(), "ok", startedAt)
	return variants
}

func executeStageTaskValue(ctx context.Context, stage Stage, asset *catalog.Asset, task Task) (any, error) {
	if batchStage, ok := stage.(BatchStage); ok {
		variants, err := batchStage.ExecuteBatch(ctx, task, asset)
		if err != nil {
			return nil, oops.Wrapf(err, "execute batch stage task")
		}
		return variants, nil
	}
	variant, err := stage.Execute(ctx, task, asset)
	if err != nil {
		return nil, oops.Wrapf(err, "execute stage task")
	}
	return cxlist.NewList(variant), nil
}

func stageTaskVariants(value any) *cxlist.List[*catalog.Variant] {
	variants, ok := value.(*cxlist.List[*catalog.Variant])
	if !ok || variants == nil {
		return cxlist.NewList[*catalog.Variant]()
	}
	return variants.Where(func(_ int, variant *catalog.Variant) bool {
		return variant != nil
	})
}

func (s *Service) recordStageTaskError(
	ctx context.Context,
	stage Stage,
	asset *catalog.Asset,
	startedAt time.Time,
	err error,
) {
	if IsVariantSkipped(err) {
		s.recordStageRunMetrics(ctx, stage.Name(), "skipped", startedAt)
		return
	}
	s.recordStageRunMetrics(ctx, stage.Name(), "error", startedAt)
	s.logStageFailure(stage, asset, err)
}

func (s *Service) logStageFailure(stage Stage, asset *catalog.Asset, err error) {
	s.logger.Error("Pipeline stage failed",
		slog.String("stage", stage.Name()),
		slog.String("asset", asset.Path),
		slog.Any("error", err),
	)
}

func (s *Service) upsertStageVariant(ctx context.Context, stage Stage, asset *catalog.Asset, variant *catalog.Variant) {
	if err := s.catalog.UpsertVariant(variant); err != nil {
		s.logger.Error("Catalog variant upsert failed",
			slog.String("stage", stage.Name()),
			slog.String("asset", asset.Path),
			slog.Any("error", err),
		)
		return
	}
	s.recordGeneratedVariantMetrics(ctx, stage.Name(), variant)
	go s.catMetrics.SyncCatalog(s.catalog)
	s.publishVariantGenerated(ctx, stage.Name(), variant)
}
