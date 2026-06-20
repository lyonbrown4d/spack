//go:build spack_libvips

package pipeline

import (
	"context"
	"log/slog"
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/observabilityx"
	"github.com/davidbyttow/govips/v2/vips"
	"github.com/lyonbrown4d/spack/internal/config"
)

func newImageEngine(
	cfg *config.Image,
	logger *slog.Logger,
	obs observabilityx.Observability,
) imageEngine {
	baseLogger := normalizeImageEngineLogger(logger)
	telemetry := newImageEngineTelemetry(baseLogger, obs)
	engine := newLibvipsImageEngine(cfg, baseLogger, telemetry)
	warnUnsupportedConfiguredImageFormats(baseLogger, cfg, engine)
	return engine
}

func stopImageEngine(context.Context, imageEngine) error {
	vips.Shutdown()
	return nil
}

func warnUnsupportedConfiguredImageFormats(logger *slog.Logger, cfg *config.Image, engine imageEngine) {
	if cfg == nil || engine == nil {
		return
	}
	formats := cfg.ParsedFormats()
	if formats == nil || formats.IsEmpty() {
		return
	}
	supported := imageFormatSet(engine.SupportedTargetFormats())
	formats.Range(func(_ int, format string) bool {
		format = strings.TrimSpace(format)
		if _, ok := supported[format]; format != "" && !ok {
			logger.Warn("Configured image output format is not supported by image engine",
				slog.String("engine", engine.Name()),
				slog.String("format", format),
			)
		}
		return true
	})
}

func imageFormatSet(formats *cxlist.List[string]) map[string]struct{} {
	supported := map[string]struct{}{}
	if formats == nil {
		return supported
	}
	formats.Range(func(_ int, format string) bool {
		supported[strings.TrimSpace(format)] = struct{}{}
		return true
	})
	return supported
}

func imageEngineMemoryBudgetLimit(cfg *config.Image) int64 {
	if cfg == nil {
		return 0
	}
	return cfg.MaxMemoryBytes
}
