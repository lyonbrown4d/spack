//go:build !libvips || !cgo

package pipeline

import (
	"context"
	"log/slog"

	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/config"
)

func newImageEngine(
	cfg *config.Image,
	logger *slog.Logger,
	obs observabilityx.Observability,
) imageEngine {
	baseLogger := normalizeImageEngineLogger(logger)
	telemetry := newImageEngineTelemetry(baseLogger, obs)
	engine := newBuiltinImageEngine(cfg, baseLogger, telemetry)
	warnUnsupportedConfiguredImageFormats(baseLogger, cfg, engine)
	return engine
}

func stopImageEngine(context.Context, imageEngine) error {
	return nil
}
