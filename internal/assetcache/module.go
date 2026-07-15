package assetcache

import (
	"context"

	"github.com/arcgolabs/dix"
)

var Module = dix.NewModule("assetcache",
	dix.WithModuleProviders(
		dix.ProviderErr6(newCache),
	),
	dix.WithModuleHooks(
		dix.OnStart(func(ctx context.Context, cache *Cache) error {
			return cache.start(ctx)
		}),
		dix.OnStop(func(ctx context.Context, cache *Cache) error {
			return cache.stop(ctx)
		}),
	),
)
