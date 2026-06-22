package assetcache

import (
	"context"
	"log/slog"
	"os"

	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/observabilityx"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/oops"
)

var assetCacheCounterSpecs = cxmapping.NewMapFrom(map[string]observabilityx.CounterSpec{
	metricAssetCacheWarmEntries: observabilityx.NewCounterSpec(
		metricAssetCacheWarmEntries,
		observabilityx.WithDescription("Total number of entries loaded into the in-memory asset cache during warmup."),
	),
	metricAssetCacheWarmBytes: observabilityx.NewCounterSpec(
		metricAssetCacheWarmBytes,
		observabilityx.WithDescription("Total number of bytes loaded into the in-memory asset cache during warmup."),
		observabilityx.WithUnit("By"),
	),
	metricAssetCacheEvictions: observabilityx.NewCounterSpec(
		metricAssetCacheEvictions,
		observabilityx.WithDescription("Total number of asset cache evictions."),
	),
	metricAssetCacheEvictedBytes: observabilityx.NewCounterSpec(
		metricAssetCacheEvictedBytes,
		observabilityx.WithDescription("Total number of bytes evicted from the in-memory asset cache."),
		observabilityx.WithUnit("By"),
	),
	metricAssetCacheHits: observabilityx.NewCounterSpec(
		metricAssetCacheHits,
		observabilityx.WithDescription("Total number of in-memory asset cache hits."),
	),
	metricAssetCacheMisses: observabilityx.NewCounterSpec(
		metricAssetCacheMisses,
		observabilityx.WithDescription("Total number of in-memory asset cache misses."),
	),
	metricAssetCacheLoadErrors: observabilityx.NewCounterSpec(
		metricAssetCacheLoadErrors,
		observabilityx.WithDescription("Total number of asset cache read or load errors."),
	),
	metricAssetCacheFills: observabilityx.NewCounterSpec(
		metricAssetCacheFills,
		observabilityx.WithDescription("Total number of cache fill operations."),
	),
	metricAssetCacheFillBytes: observabilityx.NewCounterSpec(
		metricAssetCacheFillBytes,
		observabilityx.WithDescription("Total number of bytes inserted into the in-memory asset cache."),
		observabilityx.WithUnit("By"),
	),
})

func (c *Cache) readFile(path string) ([]byte, error) {
	body, err := readResolvedAssetPath(path)
	if err != nil {
		if c.logger != nil {
			c.logger.Error("Read asset failed",
				slog.String("path", path),
				slog.String("err", err.Error()),
			)
		}
		return nil, oops.Wrapf(err, "read resolved asset")
	}
	return body, nil
}

func readResolvedAssetPath(path string) ([]byte, error) {
	if spackbundle.IsReference(path) {
		body, err := spackbundle.ReadReference(path)
		if err != nil {
			return nil, oops.Wrapf(err, "read bundle asset")
		}
		return body, nil
	}
	// #nosec G304 -- path comes from resolver/catalog-selected asset paths already validated against the asset tree.
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, oops.Wrapf(err, "read asset file")
	}
	return body, nil
}

func (c *Cache) recordWarmStats(ctx context.Context, stats WarmStats) {
	if stats.Entries == 0 {
		return
	}
	c.addCounterWithContext(ctx, metricAssetCacheWarmEntries, int64(stats.Entries))
	c.addCounterWithContext(ctx, metricAssetCacheWarmBytes, stats.Bytes)
}

func (c *Cache) onEviction(item *ristretto.Item[*Entry]) {
	c.addCounter(metricAssetCacheEvictions, 1)
	if item != nil && item.Value != nil {
		c.addCounter(metricAssetCacheEvictedBytes, int64(len(item.Value.Body)))
	}
}

func (c *Cache) addCounter(name string, value int64) {
	c.addCounterWithContext(context.Background(), name, value)
}

func (c *Cache) addCounterWithContext(ctx context.Context, name string, value int64) {
	if value == 0 || c == nil || c.obs == nil {
		return
	}
	spec, ok := assetCacheCounterSpecs.Get(name)
	if !ok {
		spec = observabilityx.NewCounterSpec(name)
	}
	c.obs.Counter(spec).Add(ctx, value)
}
