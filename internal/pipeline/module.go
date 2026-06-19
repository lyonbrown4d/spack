package pipeline

import (
	"context"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"log/slog"
)

var Module = dix.NewModule("pipeline",
	dix.WithModuleProviders(
		dix.Provider0(newMetrics),
		dix.Provider2(newImageEngine),
		dix.Provider4(newImageStage),
		dix.Provider4(newCompressionStage),
		dix.Contribute1(newImageStageRegistration, dix.Order(100)),
		dix.Contribute1(newCompressionStageRegistration, dix.Order(200)),
		dix.Provider1(newStages),
		dix.Provider6(newServiceDeps),
		dix.Provider4(newService),
	),
	dix.WithModuleHooks(
		dix.OnStart(startServiceLifecycle),
		dix.OnStop(func(ctx context.Context, svc *Service) error {
			return svc.stop(ctx)
		}),
	),
)

func newImageStageRegistration(
	stage *imageStage,
) stageRegistration {
	return newStageRegistration(100, stage)
}

func newCompressionStageRegistration(
	stage *compressionStage,
) stageRegistration {
	return newStageRegistration(200, stage)
}

func newStages(registrations *cxlist.List[stageRegistration]) *cxlist.List[Stage] {
	return buildStages(registrations)
}

type serviceDeps struct {
	metrics    *Metrics
	stages     *cxlist.List[Stage]
	bus        eventx.BusRuntime
	workers    *asyncx.Settings
	obs        observabilityx.Observability
	catMetrics *catalog.RuntimeMetrics
}

func newServiceDeps(
	metrics *Metrics,
	stages *cxlist.List[Stage],
	bus eventx.BusRuntime,
	workers *asyncx.Settings,
	obs observabilityx.Observability,
	catMetrics *catalog.RuntimeMetrics,
) serviceDeps {
	return serviceDeps{
		metrics:    metrics,
		stages:     stages,
		bus:        bus,
		workers:    workers,
		obs:        observabilityx.Normalize(obs, nil),
		catMetrics: catMetrics,
	}
}

func newService(
	cfg *config.Compression,
	logger *slog.Logger,
	cat catalog.Catalog,
	deps serviceDeps,
) *Service {
	workers := max(cfg.Workers, 1)
	queueSize := resolveQueueSize(cfg, workers)

	svc := newServiceState(serviceStateDeps{
		cfg:       cfg,
		logger:    logger,
		cat:       cat,
		services:  deps,
		queueSize: queueSize,
	})
	svc.initializeMetrics(queueSize)
	return svc
}

func startServiceLifecycle(ctx context.Context, svc *Service) error {
	workers := max(svc.cfg.Workers, 1)
	return svc.start(ctx, workers, cap(svc.tasks))
}
