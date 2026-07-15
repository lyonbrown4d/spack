package assetcache

import (
	"log/slog"

	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/observabilityx"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/cachepolicy"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/samber/oops"
	"golang.org/x/sync/singleflight"
)

type Cache struct {
	logger    *slog.Logger
	obs       observabilityx.Observability
	policy    cachepolicy.MemoryPolicy
	warmup    bool
	cache     *ristretto.Cache[string, *Entry]
	loader    singleflight.Group
	bus       eventx.BusRuntime
	workers   *asyncx.Settings
	fileGuard *source.LocalRootGuard

	variantRemovedUnsubscribe   func()
	variantGeneratedUnsubscribe func()
}

func newCache(
	cfg *config.Config,
	logger *slog.Logger,
	obs observabilityx.Observability,
	bus eventx.BusRuntime,
	workers *asyncx.Settings,
	src *source.LocalFS,
) (*Cache, error) {
	cacheCfg := cfg.HTTP.MemoryCache
	fileGuard, err := newCacheSourceGuard(src, cfg.Assets.Root)
	if err != nil {
		return nil, err
	}
	cache := &Cache{
		logger:    logger,
		obs:       observabilityx.Normalize(obs, logger),
		policy:    cachepolicy.NewMemoryPolicy(cfg),
		warmup:    cacheCfg.WarmupEnabled(),
		bus:       bus,
		workers:   workers,
		fileGuard: fileGuard,
	}
	if !cacheCfg.Enabled() {
		return cache, nil
	}

	bodyCache, err := ristretto.NewCache(&ristretto.Config[string, *Entry]{
		NumCounters:        cacheCfg.NumCounters(),
		MaxCost:            cacheCfg.MaxCost(),
		BufferItems:        64,
		IgnoreInternalCost: true,
		OnEvict:            cache.onEviction,
	})
	if err != nil {
		return nil, oops.Wrapf(err, "create asset memory cache")
	}
	cache.cache = bodyCache
	return cache, nil
}

func newCacheSourceGuard(src *source.LocalFS, fallbackRoot string) (*source.LocalRootGuard, error) {
	switch {
	case src != nil:
		guard, ok, err := src.RootGuard()
		if err != nil {
			return nil, oops.Wrapf(err, "create local source root guard")
		}
		if ok {
			return guard, nil
		}
	default:
		guard, ok, err := source.NewLocalRootGuard(fallbackRoot)
		if err != nil {
			return nil, oops.Wrapf(err, "create local source root guard")
		}
		if ok {
			return guard, nil
		}
	}
	return nil, oops.Errorf("local source root guard is required")
}
