package server

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/arcgolabs/eventx"
	appEvent "github.com/lyonbrown4d/spack/internal/event"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/samber/lo"
)

type EventPublisher struct {
	bus    eventx.BusRuntime
	logger *slog.Logger
}

func newEventPublisher(bus eventx.BusRuntime, logger *slog.Logger) *EventPublisher {
	return &EventPublisher{
		bus:    bus,
		logger: logger,
	}
}

func publishVariantServed(
	ctx context.Context,
	result *resolver.Result,
	bus eventx.BusRuntime,
	logger *slog.Logger,
) {
	if result == nil || result.Variant == nil || bus == nil {
		return
	}

	err := bus.Publish(ctx, appEvent.VariantServed{
		AssetPath:     lo.Ternary(result.Asset != nil, result.Asset.Path, ""),
		ArtifactPath:  result.FilePath,
		ServedAt:      time.Now(),
		ContentType:   result.MediaType,
		ContentCoding: result.ContentEncoding,
	})
	if shouldIgnoreVariantServedPublishError(err) {
		return
	}

	logger.Debug("Publish variant served event failed",
		slog.String("path", result.FilePath),
		slog.String("err", err.Error()),
	)
}

func shouldIgnoreVariantServedPublishError(err error) bool {
	return err == nil || errors.Is(err, eventx.ErrBusClosed)
}
