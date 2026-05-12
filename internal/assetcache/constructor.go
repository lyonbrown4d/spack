package assetcache

import (
	"fmt"
	"log/slog"

	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/observabilityx"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/cachepolicy"
	"github.com/lyonbrown4d/spack/internal/config"
)

type Cache struct {
	logger  *slog.Logger
	obs     observabilityx.Observability
	policy  cachepolicy.MemoryPolicy
	warmup  bool
	cache   *ristretto.Cache[string, []byte]
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

	bodyCache, err := ristretto.NewCache(&ristretto.Config[string, []byte]{
		NumCounters:        cacheCfg.NumCounters(),
		MaxCost:            cacheCfg.MaxCost(),
		BufferItems:        64,
		IgnoreInternalCost: true,
		OnEvict:            cache.onEviction,
	})
	if err != nil {
		return nil, fmt.Errorf("create asset memory cache: %w", err)
	}
	cache.cache = bodyCache
	return cache, nil
}
