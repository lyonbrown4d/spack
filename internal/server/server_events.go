package server

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/arcgolabs/eventx"
	appEvent "github.com/lyonbrown4d/spack/internal/event"
	"github.com/lyonbrown4d/spack/internal/resolver"
)

type EventPublisher struct {
	bus    *eventx.Bus
	logger *slog.Logger
}

func newEventPublisher(bus *eventx.Bus, logger *slog.Logger) *EventPublisher {
	return &EventPublisher{
		bus:    bus,
		logger: logger,
	}
}

func publishVariantServed(
	ctx context.Context,
	result *resolver.Result,
	bus *eventx.Bus,
	logger *slog.Logger,
) {
	if result == nil || result.Variant == nil || bus == nil {
		return
	}

	publishVariantServedLazy(ctx, bus, result.FilePath, logger, func() appEvent.VariantServed {
		assetPath := ""
		if result.Asset != nil {
			assetPath = result.Asset.Path
		}
		return appEvent.VariantServed{
			AssetPath:     assetPath,
			ArtifactPath:  result.FilePath,
			ServedAt:      time.Now(),
			ContentType:   result.MediaType,
			ContentCoding: result.ContentEncoding,
		}
	})
}

func publishVariantServedLazy(
	ctx context.Context,
	bus *eventx.Bus,
	path string,
	logger *slog.Logger,
	factory func() appEvent.VariantServed,
) {
	err := bus.PublishLazy(ctx, factory)
	if shouldIgnoreVariantServedPublishError(err) || logger == nil {
		return
	}

	logger.Debug("Publish variant served event failed",
		slog.String("path", path),
		slog.Any("error", err),
	)
}

func shouldIgnoreVariantServedPublishError(err error) bool {
	return err == nil || errors.Is(err, eventx.ErrBusClosed)
}
