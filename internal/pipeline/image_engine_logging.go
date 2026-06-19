package pipeline

import (
	"context"
	"log/slog"

	"github.com/lyonbrown4d/spack/internal/media"
)

func (engine builtinImageEngine) log() *slog.Logger {
	if engine.logger == nil {
		return normalizeImageEngineLogger(nil).With(slog.String("engine", "builtin"))
	}
	return engine.logger
}

func builtinImageBatchLogAttrs(request imageGenerateBatchRequest) []any {
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

func builtinSourceLogAttrs(source builtinSourceImage) []any {
	return []any{
		slog.Int("source_width", source.width),
		slog.Int("source_height", source.height),
		slog.Int64("source_bytes", source.bytes),
	}
}

func builtinVariantLogAttrs(variant imageVariantGenerateRequest, width int) []any {
	return []any{
		slog.String("target_format", media.NormalizeImageFormat(variant.TargetFormat)),
		slog.Int("target_width", variant.TargetWidth),
		slog.Int("normalized_width", width),
	}
}

func logBuiltinImageGenerationError(logger *slog.Logger, message string, err error, attrs []any) {
	level := slog.LevelWarn
	if IsVariantSkipped(err) {
		level = slog.LevelDebug
	}
	logger.Log(
		context.Background(),
		level,
		message,
		mergeBuiltinImageLogAttrs(attrs, slog.String("err", err.Error()))...,
	)
}

func mergeBuiltinImageLogAttrs(base []any, extra ...any) []any {
	attrs := make([]any, 0, len(base)+len(extra))
	attrs = append(attrs, base...)
	attrs = append(attrs, extra...)
	return attrs
}
