// Package task for schedule task
package task

import (
	"cmp"
	"context"
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/observabilityx"
	"github.com/go-co-op/gocron/v2"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/samber/oops"
	"log/slog"
	"strings"
	"sync"
)

var Module = dix.NewModule("task",
	dix.WithModuleProviders(
		dix.Provider0(NewRuntimeMetrics),
		dix.ProviderErr2(newScheduler),
		dix.Provider2(newTaskTelemetry),
		dix.Provider6(newSourceRescanRuntime),
		dix.Provider5(newCacheWarmerRuntime),
		dix.Provider1(newSourceRescanWatcher),
		dix.Contribute1(newSourceRescanTaskRegistration, dix.Order(100)),
		dix.Contribute1(newCacheWarmerTaskRegistration, dix.Order(300)),
	),
	dix.WithModuleHooks(
		dix.OnStart2(startTaskRuntime),
		dix.OnStart(startSourceRescanWatcher),
		dix.OnStop(stopSourceRescanWatcher),
		dix.OnStop(stopTaskRuntime),
	),
)

func newScheduler(logger *slog.Logger, runtimeMetrics *RuntimeMetrics) (gocron.Scheduler, error) {
	scheduler, err := gocron.NewScheduler(
		gocron.WithLogger(logger),
		gocron.WithSchedulerMonitor(runtimeMetrics),
	)
	if err != nil {
		return nil, oops.In("task").Owner("scheduler").Wrap(err)
	}
	return scheduler, nil
}

type taskRegistration struct {
	Order    int
	Name     string
	Register func(context.Context, gocron.Scheduler) (bool, error)
}

func newTaskRegistration(
	order int,
	name string,
	register func(context.Context, gocron.Scheduler) (bool, error),
) taskRegistration {
	return taskRegistration{
		Order:    order,
		Name:     strings.TrimSpace(name),
		Register: register,
	}
}

func newTaskRegistrations(
	registrations *cxlist.List[taskRegistration],
) *cxlist.List[taskRegistration] {
	return registrations.Clone().Sort(func(left, right taskRegistration) int {
		if left.Order != right.Order {
			return cmp.Compare(left.Order, right.Order)
		}
		return cmp.Compare(left.Name, right.Name)
	})
}

type sourceRescanRuntime struct {
	scanner    sourcecatalog.Scanner
	catalog    catalog.Catalog
	catMetrics *catalog.RuntimeMetrics
	bodyCache  *assetcache.Cache
	bus        eventx.BusRuntime
	logger     *slog.Logger
	obs        observabilityx.Observability
	rescanMu   sync.Mutex
}

type taskTelemetry struct {
	logger *slog.Logger
	obs    observabilityx.Observability
}

func newTaskTelemetry(logger *slog.Logger, obs observabilityx.Observability) taskTelemetry {
	return taskTelemetry{
		logger: logger,
		obs:    observabilityx.Normalize(obs, logger),
	}
}

func newSourceRescanRuntime(
	scanner sourcecatalog.Scanner,
	cat catalog.Catalog,
	catMetrics *catalog.RuntimeMetrics,
	bodyCache *assetcache.Cache,
	bus eventx.BusRuntime,
	telemetry taskTelemetry,
) *sourceRescanRuntime {
	return &sourceRescanRuntime{
		scanner:    scanner,
		catalog:    cat,
		catMetrics: catMetrics,
		bodyCache:  bodyCache,
		bus:        bus,
		logger:     telemetry.logger,
		obs:        telemetry.obs,
	}
}

func newSourceRescanTaskRegistration(runtime *sourceRescanRuntime) taskRegistration {
	return newTaskRegistration(100, "source_rescan", func(ctx context.Context, scheduler gocron.Scheduler) (bool, error) {
		return registerSourceRescanTask(ctx, scheduler, runtime)
	})
}

type sourceRescanWatcher struct {
	runtime *sourceRescanRuntime
	cancel  context.CancelFunc
	done    chan struct{}
}

func newSourceRescanWatcher(runtime *sourceRescanRuntime) *sourceRescanWatcher {
	return &sourceRescanWatcher{runtime: runtime}
}

type cacheWarmerRuntime struct {
	cfg       *config.Config
	catalog   catalog.Catalog
	bodyCache *assetcache.Cache
	logger    *slog.Logger
	obs       observabilityx.Observability
}

func newCacheWarmerRuntime(
	cfg *config.Config,
	cat catalog.Catalog,
	bodyCache *assetcache.Cache,
	logger *slog.Logger,
	obs observabilityx.Observability,
) *cacheWarmerRuntime {
	return &cacheWarmerRuntime{
		cfg:       cfg,
		catalog:   cat,
		bodyCache: bodyCache,
		logger:    logger,
		obs:       observabilityx.Normalize(obs, logger),
	}
}

func newCacheWarmerTaskRegistration(runtime *cacheWarmerRuntime) taskRegistration {
	return newTaskRegistration(300, "cache_warmer", func(ctx context.Context, scheduler gocron.Scheduler) (bool, error) {
		return registerCacheWarmerTask(ctx, scheduler, runtime)
	})
}

func startScheduledTasks(
	ctx context.Context,
	scheduler gocron.Scheduler,
	registrations *cxlist.List[taskRegistration],
) error {
	registered := cxlist.FilterList(newTaskRegistrations(registrations), func(_ int, registration taskRegistration) bool {
		return registration.Register != nil
	})
	if registered.IsEmpty() {
		return nil
	}

	started, err := cxlist.ReduceErrList[taskRegistration, bool](registered, false, func(started bool, _ int, registration taskRegistration) (bool, error) {
		enabled, err := registration.Register(ctx, scheduler)
		if err != nil {
			return started, oops.In("task").Owner(registration.Name).Wrap(err)
		}
		return started || enabled, nil
	})
	if err != nil {
		return err
	}
	if started {
		scheduler.Start()
	}
	return nil
}
