//go:build libvips && cgo

package pipeline

import (
	"context"
	"log/slog"

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
