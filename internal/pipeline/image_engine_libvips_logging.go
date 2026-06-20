//go:build spack_libvips

package pipeline

import "log/slog"

func logCompletedLibvipsBatch(
	logger *slog.Logger,
	batchAttrs []any,
	source libvipsSourceImage,
	generatedVariants int,
) {
	logger.Debug("Libvips image generation completed",
		mergeImageLogAttrs(batchAttrs,
			slog.Int("generated_variants", generatedVariants),
			slog.Int("source_width", source.width),
			slog.Int("source_height", source.height),
			slog.Int64("source_bytes", source.bytes),
		)...,
	)
}
