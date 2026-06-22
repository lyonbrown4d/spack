package assetcache

import (
	"log/slog"

	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/observabilityx"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/cachepolicy"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/samber/oops"
	"golang.org/x/sync/singleflight"
)

type Cache struct {
	logger  *slog.Logger
	obs     observabilityx.Observability
	policy  cachepolicy.MemoryPolicy
	warmup  bool
	cache   *ristretto.Cache[string, *Entry]
	loader  singleflight.Group
	bus     eventx.BusRuntime
	workers *asyncx.Settings

	variantRemovedUnsubscribe   func()
	variantGeneratedUnsubscribe func()
}

func newCache(
	cfg *config.Config,
	logger *slog.Logger,
	obs observabilityx.Observability,
	bus eventx.BusRuntime,
	workers *asyncx.Settings,
) (*Cache, error) {
	cacheCfg := cfg.HTTP.MemoryCache
	cache := &Cache{
		logger:  logger,
		obs:     observabilityx.Normalize(obs, logger),
		policy:  cachepolicy.NewMemoryPolicy(cfg),
		warmup:  cacheCfg.WarmupEnabled(),
		bus:     bus,
		workers: workers,
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
