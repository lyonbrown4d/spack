package pipeline

import (
	"log/slog"
	"strings"

	"github.com/lyonbrown4d/spack/internal/config"
)

func newBuiltinImageEngine(
	cfg *config.Image,
	logger *slog.Logger,
	telemetry imageEngineTelemetry,
) imageEngine {
	logger.Info("Image engine configured", slog.String("engine", "builtin"))
	return builtinImageEngine{
		logger:    logger.With(slog.String("engine", "builtin")),
		telemetry: telemetry,
		memory:    newImageMemoryBudget(imageEngineMemoryBudgetLimit(cfg)),
	}
}

func warnUnsupportedConfiguredImageFormats(logger *slog.Logger, cfg *config.Image, engine imageEngine) {
	if cfg == nil || engine == nil {
		return
	}
	formats := cfg.ParsedFormats()
	if formats == nil || formats.IsEmpty() {
		return
	}
	supported := map[string]struct{}{}
	engine.SupportedTargetFormats().Range(func(_ int, format string) bool {
		supported[strings.TrimSpace(format)] = struct{}{}
		return true
	})
	formats.Range(func(_ int, format string) bool {
		format = strings.TrimSpace(format)
		if _, ok := supported[format]; format != "" && !ok {
			logger.Warn("Configured image output format is not supported by active image engine",
				slog.String("engine", engine.Name()),
				slog.String("format", format),
			)
		}
		return true
	})
}

func imageEngineMemoryBudgetLimit(cfg *config.Image) int64 {
	if cfg == nil {
		return 0
	}
	return cfg.MaxMemoryBytes
}
