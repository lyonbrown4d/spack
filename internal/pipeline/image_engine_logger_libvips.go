//go:build spack_libvips

package pipeline

import "log/slog"

func normalizeImageEngineLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default().WithGroup("image_engine")
	}
	return logger.WithGroup("image_engine")
}
