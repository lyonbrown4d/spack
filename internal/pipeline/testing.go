package pipeline

import (
	"context"
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/artifact"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"log/slog"
	"time"
)

// NewCompressionStageForTest exposes compression stage construction for external tests.
func NewCompressionStageForTest(cfg *config.Compression, store artifact.Store, cat catalog.Catalog) Stage {
	return newCompressionStage(
		cfg,
		contentcoding.NewRegistry(contentcoding.Options{
			BrotliQuality: cfg.BrotliQuality,
			GzipLevel:     cfg.GzipLevel,
			ZstdLevel:     cfg.ZstdLevel,
		}, cfg.NormalizedEncodings()),
		store,
		cat,
	)
}

// NewImageStageForTest exposes image stage construction for external tests.
func NewImageStageForTest(cfg *config.Image, store artifact.Store, cat catalog.Catalog) Stage {
	return newImageStage(cfg, newImageEngine(), store, cat)
}

// NormalizeEncodingsForTest exposes compression encoding normalization for external tests.
func NormalizeEncodingsForTest(values *cxlist.List[string]) *cxlist.List[string] {
	return normalizeEncodings(values)
}

// NormalizeImageFormatsForTest exposes image format normalization for external tests.
func NormalizeImageFormatsForTest(values *cxlist.List[string]) *cxlist.List[string] {
	return normalizeImageFormats(values)
}

// NormalizeRequestStringsForTest exposes request string normalization for external tests.
func NormalizeRequestStringsForTest(values *cxlist.List[string]) *cxlist.List[string] {
	return normalizeRequestStrings(values)
}

// NormalizeRequestIntsForTest exposes request integer normalization for external tests.
func NormalizeRequestIntsForTest(values *cxlist.List[int]) *cxlist.List[int] {
	return normalizeRequestInts(values)
}

// NewServiceForTest exposes service construction for external tests.
func NewServiceForTest(cfg *config.Compression, logger *slog.Logger, cat catalog.Catalog, queueSize int) *Service {
	return newServiceState(serviceStateDeps{
		cfg:    cfg,
		logger: logger,
		cat:    cat,
		services: serviceDeps{
			obs: observabilityx.NopWithLogger(logger),
		},
		queueSize: queueSize,
	})
}

// NewServiceWithBusForTest exposes service construction with an event bus for external tests.
func NewServiceWithBusForTest(
	cfg *config.Compression,
	logger *slog.Logger,
	cat catalog.Catalog,
	bus eventx.BusRuntime,
	queueSize int,
) *Service {
	return newServiceState(serviceStateDeps{
		cfg:    cfg,
		logger: logger,
		cat:    cat,
		services: serviceDeps{
			bus: bus,
			obs: observabilityx.NopWithLogger(logger),
		},
		queueSize: queueSize,
	})
}

// NewServiceWithObservabilityForTest exposes service construction with an observability backend for external tests.
func NewServiceWithObservabilityForTest(
	cfg *config.Compression,
	logger *slog.Logger,
	cat catalog.Catalog,
	obs observabilityx.Observability,
	queueSize int,
) *Service {
	return newServiceState(serviceStateDeps{
		cfg:    cfg,
		logger: logger,
		cat:    cat,
		services: serviceDeps{
			obs: obs,
		},
		queueSize: queueSize,
	})
}

type testStage struct {
	name string
}

func (s testStage) Name() string {
	return s.name
}

func (testStage) Plan(_ *catalog.Asset, _ Request) *cxlist.List[Task] {
	return nil
}

func (testStage) Execute(_ Task, _ *catalog.Asset) (*catalog.Variant, error) {
	return nil, ErrVariantSkipped
}

// PendingCountForTest exposes pending queue cardinality for external tests.
func PendingCountForTest(s *Service) int {
	return s.pending.Len()
}

// QueuedCountForTest exposes queued request count for external tests.
func QueuedCountForTest(s *Service) int {
	return len(s.tasks)
}

// DequeueRequestForTest exposes draining the queued request channel for external tests.
func DequeueRequestForTest(s *Service) Request {
	return <-s.tasks
}

// FinishRequestForTest exposes pending-set cleanup for a queued request in external tests.
func FinishRequestForTest(s *Service, request Request) {
	s.finishRequest(requestKey(request))
}

// CleanupRemovedForTest exposes cleanup execution for external tests.
func CleanupRemovedForTest(s *Service, now time.Time) int {
	return s.cleanupArtifacts(context.Background(), now).removed
}

// SubscribeVariantServedForTest exposes event subscription for external tests.
func SubscribeVariantServedForTest(s *Service) error {
	return s.subscribeVariantServed()
}

// UpsertStageVariantForTest exposes catalog upsert and side effects for external tests.
func UpsertStageVariantForTest(s *Service, stageName string, asset *catalog.Asset, variant *catalog.Variant) {
	s.upsertStageVariant(context.Background(), testStage{name: stageName}, asset, variant)
}

// ExecuteStageTaskForTest exposes stage execution for external tests.
func ExecuteStageTaskForTest(s *Service, stage Stage, asset *catalog.Asset, task Task) *catalog.Variant {
	return s.executeStageTask(context.Background(), stage, asset, task)
}
