//go:build !spack_libvips

package pipeline

import (
	"context"
	"log/slog"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/config"
)

func newImageEngine(cfg *config.Image, logger *slog.Logger, obs observabilityx.Observability) imageEngine {
	_ = obs

	engineLogger := imageEngineDisabledLogger(logger)
	if cfg != nil && cfg.Enable {
		engineLogger.Warn(
			"Image variant pipeline is disabled in this binary",
			slog.String("required_build_tag", "spack_libvips"),
			slog.String("hint", "use spack-compiler or rebuild with -tags=spack_libvips"),
		)
	}

	return disabledImageEngine{logger: engineLogger}
}

func stopImageEngine(context.Context, imageEngine) error {
	return nil
}

type disabledImageEngine struct {
	logger *slog.Logger
}

func (engine disabledImageEngine) Name() string {
	return "disabled"
}

func (engine disabledImageEngine) SupportsSourceMediaType(string) bool {
	return false
}

func (engine disabledImageEngine) SupportedTargetFormats() *cxlist.List[string] {
	return cxlist.NewList[string]()
}

func (engine disabledImageEngine) Generate(request imageGenerateRequest) (imageGenerateResult, error) {
	engine.logger.Debug(
		"Image variant generation skipped because no image engine is linked",
		slog.String("source_path", request.SourcePath),
		slog.String("target_format", request.TargetFormat),
		slog.Int("target_width", request.TargetWidth),
	)
	return imageGenerateResult{}, ErrVariantSkipped
}

func (engine disabledImageEngine) GenerateBatch(request imageGenerateBatchRequest) (*cxlist.List[imageGenerateResult], error) {
	engine.logger.Debug(
		"Image variant batch skipped because no image engine is linked",
		slog.String("source_path", request.SourcePath),
		slog.Int("variant_count", imageVariantGenerateRequestCount(request.Variants)),
	)
	return cxlist.NewList[imageGenerateResult](), ErrVariantSkipped
}

func imageEngineDisabledLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return logger.With(
		slog.String("component", "image_engine"),
		slog.String("engine", "disabled"),
	)
}

func imageVariantGenerateRequestCount(variants *cxlist.List[imageVariantGenerateRequest]) int {
	if variants == nil {
		return 0
	}
	return variants.Len()
}
