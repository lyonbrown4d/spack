package assetcache

import (
	"context"
	"time"

	"github.com/arcgolabs/observabilityx"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/oops"
)

var (
	assetCacheWarmEntriesSpec = observabilityx.NewCounterSpec(
		metricAssetCacheWarmEntries,
		observabilityx.WithDescription("Total number of entries loaded into the in-memory asset cache during warmup."),
	)
	assetCacheWarmBytesSpec = observabilityx.NewCounterSpec(
		metricAssetCacheWarmBytes,
		observabilityx.WithDescription("Total number of bytes loaded into the in-memory asset cache during warmup."),
		observabilityx.WithUnit("By"),
	)
	assetCacheEvictionsSpec = observabilityx.NewCounterSpec(
		metricAssetCacheEvictions,
		observabilityx.WithDescription("Total number of asset cache evictions."),
	)
	assetCacheEvictedBytesSpec = observabilityx.NewCounterSpec(
		metricAssetCacheEvictedBytes,
		observabilityx.WithDescription("Total number of bytes evicted from the in-memory asset cache."),
		observabilityx.WithUnit("By"),
	)
	assetCacheHitsSpec = observabilityx.NewCounterSpec(
		metricAssetCacheHits,
		observabilityx.WithDescription("Total number of in-memory asset cache hits."),
	)
	assetCacheMissesSpec = observabilityx.NewCounterSpec(
		metricAssetCacheMisses,
		observabilityx.WithDescription("Total number of in-memory asset cache misses."),
	)
	assetCacheLoadErrorsSpec = observabilityx.NewCounterSpec(
		metricAssetCacheLoadErrors,
		observabilityx.WithDescription("Total number of asset cache read or load errors."),
	)
	assetCacheFillsSpec = observabilityx.NewCounterSpec(
		metricAssetCacheFills,
		observabilityx.WithDescription("Total number of cache fill operations."),
	)
	assetCacheFillBytesSpec = observabilityx.NewCounterSpec(
		metricAssetCacheFillBytes,
		observabilityx.WithDescription("Total number of bytes inserted into the in-memory asset cache."),
		observabilityx.WithUnit("By"),
	)
	assetCacheWarmupDurationSpec = observabilityx.NewHistogramSpec(
		metricAssetCacheWarmupDuration,
		observabilityx.WithDescription("Duration of in-memory asset cache warmup runs in seconds."),
		observabilityx.WithUnit("s"),
	)
)

type assetCacheMetrics struct {
	warmEntries    observabilityx.Counter
	warmBytes      observabilityx.Counter
	evictions      observabilityx.Counter
	evictedBytes   observabilityx.Counter
	hits           observabilityx.Counter
	misses         observabilityx.Counter
	loadErrors     observabilityx.Counter
	fills          observabilityx.Counter
	fillBytes      observabilityx.Counter
	warmupDuration observabilityx.Histogram
}

func newAssetCacheMetrics(obs observabilityx.Observability) assetCacheMetrics {
	return assetCacheMetrics{
		warmEntries:    obs.Counter(assetCacheWarmEntriesSpec),
		warmBytes:      obs.Counter(assetCacheWarmBytesSpec),
		evictions:      obs.Counter(assetCacheEvictionsSpec),
		evictedBytes:   obs.Counter(assetCacheEvictedBytesSpec),
		hits:           obs.Counter(assetCacheHitsSpec),
		misses:         obs.Counter(assetCacheMissesSpec),
		loadErrors:     obs.Counter(assetCacheLoadErrorsSpec),
		fills:          obs.Counter(assetCacheFillsSpec),
		fillBytes:      obs.Counter(assetCacheFillBytesSpec),
		warmupDuration: obs.Histogram(assetCacheWarmupDurationSpec),
	}
}

func (c *Cache) readFile(path string) ([]byte, error) {
	body, err := readResolvedAssetPath(path, c.files)
	if err != nil {
		return nil, oops.With("path", path).Wrapf(err, "read resolved asset")
	}
	return body, nil
}

func readResolvedAssetPath(path string, files *source.LocalFS) ([]byte, error) {
	if spackbundle.IsReference(path) {
		body, err := spackbundle.ReadReference(path)
		if err != nil {
			return nil, oops.Wrapf(err, "read bundle asset")
		}
		return body, nil
	}
	if files == nil {
		return nil, oops.Errorf("local file source is required for %s", path)
	}
	body, err := files.ReadFile(path)
	if err != nil {
		return nil, oops.Wrapf(err, "read local asset file")
	}
	return body, nil
}

func (c *Cache) recordWarmStats(ctx context.Context, stats WarmStats, duration time.Duration) {
	c.recordHistogramWithContext(ctx, c.metrics.warmupDuration, duration.Seconds())
	if stats.Entries == 0 {
		return
	}
	c.addCounterWithContext(ctx, c.metrics.warmEntries, int64(stats.Entries))
	c.addCounterWithContext(ctx, c.metrics.warmBytes, stats.Bytes)
}

func (c *Cache) onEviction(item *ristretto.Item[*Entry]) {
	c.addCounter(c.metrics.evictions, 1)
	if item != nil && item.Value != nil {
		c.addCounter(c.metrics.evictedBytes, int64(len(item.Value.Body)))
	}
}

func (c *Cache) addCounter(counter observabilityx.Counter, value int64) {
	c.addCounterWithContext(context.TODO(), counter, value)
}

func (c *Cache) addCounterWithContext(ctx context.Context, counter observabilityx.Counter, value int64) {
	if value == 0 || c == nil || c.obs == nil || counter == nil {
		return
	}
	counter.Add(ctx, value)
}

func (c *Cache) recordHistogramWithContext(
	ctx context.Context,
	histogram observabilityx.Histogram,
	value float64,
) {
	if c == nil || c.obs == nil || histogram == nil {
		return
	}
	if value < 0 {
		value = 0
	}
	histogram.Record(ctx, value)
}
