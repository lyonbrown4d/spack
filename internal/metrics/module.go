// Package metrics exposes metrics-related services.
package metrics

import (
	"log/slog"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/observabilityx"
	obsprom "github.com/arcgolabs/observabilityx/prometheus"
	"github.com/prometheus/client_golang/prometheus"
)

var Module = dix.NewModule("metrics",
	dix.WithModuleProviders(
		dix.Provider1(NewAdapter),
		dix.Provider1(func(adapter *obsprom.Adapter) observabilityx.Observability {
			return adapter
		}),
		dix.Provider1(func(obs observabilityx.Observability) *cxlist.List[dix.Observer] {
			return cxlist.NewList[dix.Observer](NewObserver(obs))
		}),
	),
)

func NewAdapter(logger *slog.Logger) *obsprom.Adapter {
	return newAdapter(logger, prometheus.DefaultRegisterer)
}

func newAdapter(logger *slog.Logger, registerer prometheus.Registerer) *obsprom.Adapter {
	return obsprom.New(
		obsprom.WithNamespace("spack"),
		obsprom.WithLogger(logger),
		obsprom.WithRegisterer(registerer),
	)
}
