//go:build libvips && cgo

package pipeline

import (
	"context"
	"log/slog"
)

func logLibvipsImageGenerationError(logger *slog.Logger, message string, err error, attrs []any) {
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

func logCompletedLibvipsBatch(
	logger *slog.Logger,
	batchAttrs []any,
	source libvipsSourceImage,
	generatedVariants int,
) {
	logger.Debug("Libvips image generation completed",
		mergeBuiltinImageLogAttrs(batchAttrs,
			slog.Int("generated_variants", generatedVariants),
			slog.Int("source_width", source.width),
			slog.Int("source_height", source.height),
			slog.Int64("source_bytes", source.bytes),
		)...,
	)
}
