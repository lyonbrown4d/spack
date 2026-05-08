// Package event wires application event infrastructure.
package event

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/observabilityx"
	"github.com/daiyuang/spack/internal/asyncx"
	"github.com/samber/oops"
)

var Module = dix.NewModule("event",
	dix.WithModuleProviders(
		dix.ProviderErr3(newBus),
	),
	dix.WithModuleHooks(
		dix.OnStop(func(ctx context.Context, bus eventx.BusRuntime) error {
			return bus.Close()
		}),
	),
)

func newBus(
	settings *asyncx.Settings,
	logger *slog.Logger,
	obs observabilityx.Observability,
) (eventx.BusRuntime, error) {
	if settings == nil || settings.Size <= 0 {
		return nil, oops.In("event").Wrap(fmt.Errorf("invalid async worker settings"))
	}
	if logger == nil {
		return nil, oops.In("event").Owner("logger").Wrap(fmt.Errorf("logger is required"))
	}
	return eventx.New(
		eventx.WithParallelDispatch(true),
		eventx.WithAntsPool(settings.Size),
		eventx.WithObservability(obs),
		eventx.WithAsyncErrorHandler(func(_ context.Context, event eventx.Event, err error) {
			if err == nil {
				return
			}
			logger.Warn("event dispatch failed",
				slog.String("event_type", reflect.TypeOf(event).String()),
				slog.String("error", err.Error()),
			)
		}),
	), nil
}
