package pipeline

import (
	"context"
	"fmt"
	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	cxset "github.com/arcgolabs/collectionx/set"
	"github.com/arcgolabs/observabilityx"
	"github.com/daiyuang/spack/internal/cachepolicy"
	"github.com/daiyuang/spack/internal/catalog"
	"github.com/daiyuang/spack/internal/config"
	"log/slog"
	"os"
	"strconv"
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
	queueSize := cfg.QueueCapacity()
	if queueSize < 1 {
		return workers * 64
	}
	return queueSize
}

type serviceStateDeps struct {
	cfg       *config.Compression
	logger    *slog.Logger
	cat       catalog.Catalog
	services  serviceDeps
	queueSize int
}

func newServiceState(deps serviceStateDeps) *Service {
	stages := deps.services.stages
	if stages == nil {
		stages = cxlist.NewList[Stage]()
	}
	return &Service{
		cfg:            deps.cfg,
		logger:         deps.logger,
		catalog:        deps.cat,
		metrics:        deps.services.metrics,
		obs:            observabilityx.Normalize(deps.services.obs, deps.logger),
		catMetrics:     deps.services.catMetrics,
		stages:         stages,
		bus:            deps.services.bus,
		tasks:          make(chan Request, deps.queueSize),
		pending:        cxset.NewConcurrentSetWithCapacity[string](deps.queueSize),
		variantHits:    cxmapping.NewConcurrentMapWithCapacity[string, time.Time](deps.queueSize),
		warmWorkers:    deps.services.workers,
		artifactPolicy: cachepolicy.NewArtifactPolicy(deps.cfg),
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
		return fmt.Errorf("subscribe variant served events: %w", err)
	}
	if strings.TrimSpace(s.cfg.CacheDir) == "" {
		return nil
	}
	if err := os.MkdirAll(s.cfg.CacheDir, 0o750); err != nil {
		return fmt.Errorf("create pipeline cache directory: %w", err)
	}

	s.startWorkers(ctx, workers)
	s.logWorkersStarted(workers, queueSize)
	s.startCleanupIfNeeded(ctx)
	return nil
}

func (s *Service) startWorkers(ctx context.Context, workers int) {
	for range workers {
		s.wg.Go(func() {
			for request := range s.tasks {
				key := requestKey(request)
				s.updateQueueLengthMetric()
				s.process(ctx, request)
				s.finishRequest(key)
			}
		})
	}
}

func (s *Service) logWorkersStarted(workers, queueSize int) {
	s.logger.Info("Pipeline workers started",
		slog.Int("workers", workers),
		slog.Int("queue_size", queueSize),
		slog.String("mode", s.cfg.NormalizedMode()),
		slog.String("cache_dir", s.cfg.CacheDir),
	)
}

func (s *Service) startCleanupIfNeeded(ctx context.Context) {
	if !s.cleanupEnabled() {
		return
	}

	interval := s.cfg.ParsedCleanupInterval()
	s.cleanupStop = make(chan struct{})
	s.cleanupDone = make(chan struct{})
	go s.cleanupLoop(ctx, interval)
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
	if s.cleanupStop == nil {
		return nil
	}

	close(s.cleanupStop)
	select {
	case <-s.cleanupDone:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for cleanup shutdown: %w", ctx.Err())
	}
}

func (s *Service) stopWorkers(ctx context.Context) error {
	close(s.tasks)
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for worker shutdown: %w", ctx.Err())
	}
}

func (s *Service) executeStageTask(ctx context.Context, stage Stage, asset *catalog.Asset, task Task) *catalog.Variant {
	startedAt := time.Now()
	key := buildStageTaskKey(stage, asset, task)
	variantValue, err, _ := s.sf.Do(key, func() (any, error) {
		return stage.Execute(task, asset)
	})
	if err != nil {
		if IsVariantSkipped(err) {
			s.recordStageRunMetrics(ctx, stage.Name(), "skipped", startedAt)
			return nil
		}
		s.recordStageRunMetrics(ctx, stage.Name(), "error", startedAt)
		s.logStageFailure(stage, asset, err)
		return nil
	}

	variant, ok := variantValue.(*catalog.Variant)
	if !ok || variant == nil {
		s.recordStageRunMetrics(ctx, stage.Name(), "empty", startedAt)
		return nil
	}
	s.recordStageRunMetrics(ctx, stage.Name(), "ok", startedAt)
	return variant
}

func buildStageTaskKey(stage Stage, asset *catalog.Asset, task Task) string {
	return cxlist.NewList(
		stage.Name(),
		asset.Path,
		asset.SourceHash,
		task.Encoding,
		task.Format,
		strconv.Itoa(task.Width),
	).Join("|")
}

func (s *Service) logStageFailure(stage Stage, asset *catalog.Asset, err error) {
	s.logger.Error("Pipeline stage failed",
		slog.String("stage", stage.Name()),
		slog.String("asset", asset.Path),
		slog.String("err", err.Error()),
	)
}

func (s *Service) upsertStageVariant(ctx context.Context, stage Stage, asset *catalog.Asset, variant *catalog.Variant) {
	if err := s.catalog.UpsertVariant(variant); err != nil {
		s.logger.Error("Catalog variant upsert failed",
			slog.String("stage", stage.Name()),
			slog.String("asset", asset.Path),
			slog.String("err", err.Error()),
		)
		return
	}
	s.recordGeneratedVariantMetrics(ctx, stage.Name(), variant)
	go s.catMetrics.SyncCatalog(s.catalog)
	s.publishVariantGenerated(ctx, stage.Name(), variant)
}

func (s *Service) recordStageRunMetrics(ctx context.Context, stageName, result string, startedAt time.Time) {
	if s == nil || s.obs == nil {
		return
	}
	attrs := []observabilityx.Attribute{
		observabilityx.String("stage", strings.TrimSpace(stageName)),
		observabilityx.String("result", strings.TrimSpace(result)),
	}
	s.obs.Counter(pipelineStageRunsTotalSpec).Add(ctx, 1, attrs...)
	s.obs.Histogram(pipelineStageDurationSpec).Record(ctx, time.Since(startedAt).Seconds(), attrs...)
}

func (s *Service) recordGeneratedVariantMetrics(ctx context.Context, stageName string, variant *catalog.Variant) {
	if s == nil || s.obs == nil || variant == nil {
		return
	}
	attrs := []observabilityx.Attribute{
		observabilityx.String("stage", strings.TrimSpace(stageName)),
	}
	s.obs.Counter(pipelineVariantsGeneratedTotalSpec).Add(ctx, 1, attrs...)
	if variant.Size > 0 {
		s.obs.Counter(pipelineVariantsGeneratedBytesTotalSpec).Add(ctx, variant.Size, attrs...)
	}
}
