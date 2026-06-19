package pipeline

import (
	"context"
	"strings"
	"time"

	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/catalog"
)

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
