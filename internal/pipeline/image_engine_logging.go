//go:build spack_libvips

package pipeline

import (
	"context"
	"log/slog"

	"github.com/lyonbrown4d/spack/internal/media"
	"github.com/samber/lo"
)

func imageBatchLogAttrs(request imageGenerateBatchRequest) []any {
	variants := 0
	if request.Variants != nil {
		variants = request.Variants.Len()
	}
	return []any{
		slog.String("source_path", request.SourcePath),
		slog.String("source_media_type", request.SourceMediaType),
		slog.Int("requested_variants", variants),
	}
}

func imageVariantLogAttrs(variant imageVariantGenerateRequest, width int) []any {
	return []any{
		slog.String("target_format", media.NormalizeImageFormat(variant.TargetFormat)),
		slog.Int("target_width", variant.TargetWidth),
		slog.Int("normalized_width", width),
	}
}

func logImageGenerationError(logger *slog.Logger, message string, err error, attrs []any) {
	level := slog.LevelWarn
	if IsVariantSkipped(err) {
		level = slog.LevelDebug
	}
	logger.Log(
		context.Background(),
		level,
		message,
		mergeImageLogAttrs(attrs, slog.String("err", err.Error()))...,
	)
}

func mergeImageLogAttrs(base []any, extra ...any) []any {
	return lo.Concat(base, extra)
}
