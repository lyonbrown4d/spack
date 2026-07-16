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
	logger  *slog.Logger
	obs     observabilityx.Observability
	policy  cachepolicy.MemoryPolicy
	warmup  bool
	cache   *ristretto.Cache[string, *Entry]
	loader  singleflight.Group
	bus     eventx.BusRuntime
	workers *asyncx.Settings
	files   *source.LocalFS

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
	files, err := newCacheFileSource(src, cfg.Assets.Root)
	if err != nil {
		return nil, err
	}
	cache := &Cache{
		logger:  logger,
		obs:     observabilityx.Normalize(obs, logger),
		policy:  cachepolicy.NewMemoryPolicy(cfg),
		warmup:  cacheCfg.WarmupEnabled(),
		bus:     bus,
		workers: workers,
		files:   files,
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

func newCacheFileSource(src *source.LocalFS, fallbackRoot string) (*source.LocalFS, error) {
	switch {
	case src != nil:
		if src.Root() != "" {
			return src, nil
		}
	default:
		files, ok, err := source.NewLocalDirectory(fallbackRoot)
		if err != nil {
			return nil, oops.Wrapf(err, "create local cache file source")
		}
		if ok {
			return files, nil
		}
	}
	return nil, oops.Errorf("local cache file source is required")
}
