package task

import (
	"context"
	"log/slog"
	"time"

	"github.com/arcgolabs/eventx"
	appEvent "github.com/lyonbrown4d/spack/internal/event"
)

func publishCatalogChanged(ctx context.Context, bus eventx.BusRuntime, reason string, logger *slog.Logger) {
	if bus == nil {
		return
	}
	if err := bus.Publish(ctx, appEvent.CatalogChanged{
		Reason:    reason,
		ChangedAt: time.Now(),
	}); err != nil && logger != nil {
		logger.Debug("Publish catalog changed event failed",
			slog.String("reason", reason),
			slog.Any("error", err),
		)
	}
}
