// Package assetcache caches hot static asset bodies in memory for small-file delivery.
package assetcache

import (
	"context"
	"errors"
	"fmt"
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/cachepolicy"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"strings"
	"sync"
)

type WarmStats struct {
	Entries int
	Bytes   int64
}

func (c *Cache) Enabled() bool {
	return c != nil && c.cache != nil
}

func (c *Cache) WarmupEnabled() bool {
	return c.Enabled() && c.warmup
}

func (c *Cache) ShouldServe(size int64, rangeRequested bool) bool {
	return c.ShouldServeRequest(cachepolicy.MemoryRequest{
		Size:           size,
		RangeRequested: rangeRequested,
		UseCase:        cachepolicy.MemoryUseCaseDirect,
	})
}

func (c *Cache) ShouldServeRequest(request cachepolicy.MemoryRequest) bool {
	if !c.Enabled() || c.policy == nil {
		return false
	}
	return c.policy.ShouldServe(request)
}

func (c *Cache) GetOrLoad(path string) ([]byte, bool, error) {
	return c.GetOrLoadWithRequest(path, cachepolicy.MemoryRequest{
		Path:    path,
		UseCase: cachepolicy.MemoryUseCaseDirect,
	})
}

func (c *Cache) GetOrLoadWithRequest(path string, request cachepolicy.MemoryRequest) ([]byte, bool, error) {
	if !c.Enabled() {
		return nil, false, errors.New("memory cache is disabled")
	}

	if body, found := c.cache.Get(path); found {
		c.addCounter(metricAssetCacheHits, 1)
		return body, true, nil
	}
	c.addCounter(metricAssetCacheMisses, 1)

	body, cached, err := c.readAndCachePath(path, request)
	if err != nil {
		c.addCounter(metricAssetCacheLoadErrors, 1)
		return nil, false, err
	}

	if cached {
		c.addCounter(metricAssetCacheFills, 1)
		c.addCounter(metricAssetCacheFillBytes, int64(len(body)))
	}
	return body, false, nil
}

func (c *Cache) Delete(path string) bool {
	if !c.Enabled() {
		return false
	}
	_, found := c.cache.Get(path)
	if found {
		c.cache.Del(path)
		c.cache.Wait()
	}
	return found
}

func (c *Cache) WarmSelected(ctx context.Context, cat catalog.Catalog) (WarmStats, error) {
	if !c.Enabled() {
		return WarmStats{}, nil
	}

	stats := WarmStats{}
	if err := c.warmAssets(ctx, cat, &stats); err != nil {
		return WarmStats{}, fmt.Errorf("warm selected memory cache: %w", err)
	}
	c.recordWarmStats(ctx, stats)
	return stats, nil
}

func (c *Cache) Warm(ctx context.Context, cat catalog.Catalog) (WarmStats, error) {
	if !c.WarmupEnabled() {
		return WarmStats{}, nil
	}

	stats := WarmStats{}
	if err := c.warmAssets(ctx, cat, &stats); err != nil {
		return WarmStats{}, fmt.Errorf("warm memory cache: %w", err)
	}
	c.recordWarmStats(ctx, stats)

	return stats, nil
}

func (c *Cache) warmAssets(ctx context.Context, cat catalog.Catalog, stats *WarmStats) error {
	var statsMu sync.Mutex
	err := asyncx.RunList[*catalog.Asset](ctx, c.obs, c.workers, "asset_cache_warm", cat.AllAssets(), func(ctx context.Context, asset *catalog.Asset) error {
		assetStats := WarmStats{}
		if err := c.warmAsset(ctx, cat, asset, &assetStats); err != nil {
			return err
		}
		if assetStats.Entries == 0 && assetStats.Bytes == 0 {
			return nil
		}
		statsMu.Lock()
		stats.Entries += assetStats.Entries
		stats.Bytes += assetStats.Bytes
		statsMu.Unlock()
		return nil
	})
	if err != nil {
		return fmt.Errorf("run asset warm list: %w", err)
	}
	return nil
}

func (c *Cache) warmAsset(
	ctx context.Context,
	cat catalog.Catalog,
	asset *catalog.Asset,
	stats *WarmStats,
) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	if err := c.preloadAsset(asset, stats); err != nil {
		return err
	}
	return c.warmVariants(ctx, cat.ListVariants(asset.Path), stats)
}

func (c *Cache) warmVariants(
	ctx context.Context,
	variants *cxlist.List[*catalog.Variant],
	stats *WarmStats,
) error {
	var warmErr error
	variants.Range(func(_ int, variant *catalog.Variant) bool {
		warmErr = c.warmVariant(ctx, variant, stats)
		return warmErr == nil
	})
	return pickWarmError(ctx, warmErr)
}

func (c *Cache) warmVariant(ctx context.Context, variant *catalog.Variant, stats *WarmStats) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	return c.preloadVariant(variant, cachepolicy.MemoryUseCaseWarm, stats)
}

func pickWarmError(ctx context.Context, warmErr error) error {
	if warmErr != nil {
		return warmErr
	}
	return contextErr(ctx)
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context done: %w", err)
	}
	return nil
}

func (c *Cache) preloadAsset(asset *catalog.Asset, stats *WarmStats) error {
	if asset == nil {
		return nil
	}
	return c.preloadPath(asset.FullPath, cachepolicy.MemoryRequest{
		Path:      asset.FullPath,
		AssetPath: asset.Path,
		Size:      asset.Size,
		MediaType: asset.MediaType,
		Kind:      cachepolicy.MemoryEntryKindAsset,
		UseCase:   cachepolicy.MemoryUseCaseWarm,
	}, stats)
}

func (c *Cache) preloadVariant(variant *catalog.Variant, useCase cachepolicy.MemoryUseCase, stats *WarmStats) error {
	if variant == nil {
		return nil
	}
	return c.preloadPath(variant.ArtifactPath, cachepolicy.MemoryRequest{
		Path:      variant.ArtifactPath,
		AssetPath: variant.AssetPath,
		Size:      variant.Size,
		MediaType: variant.MediaType,
		Encoding:  variant.Encoding,
		Format:    variant.Format,
		Width:     variant.Width,
		Kind:      cachepolicy.MemoryEntryKindVariant,
		UseCase:   useCase,
	}, stats)
}

func (c *Cache) preloadPath(path string, request cachepolicy.MemoryRequest, stats *WarmStats) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if !c.ShouldServeRequest(request) || !c.shouldWarm(request) {
		return nil
	}
	if _, found := c.cache.Get(path); found {
		return nil
	}

	body, cached, err := c.readAndCachePath(path, request)
	if err != nil {
		return err
	}

	if stats != nil && cached {
		stats.Entries++
		stats.Bytes += int64(len(body))
	}
	return nil
}

func (c *Cache) shouldWarm(request cachepolicy.MemoryRequest) bool {
	return c.Enabled() && c.policy != nil && c.policy.ShouldWarm(request)
}

func (c *Cache) readAndCachePath(path string, request cachepolicy.MemoryRequest) ([]byte, bool, error) {
	body, err := c.readFile(path)
	if err != nil {
		return nil, false, err
	}

	ttl := c.policy.TTL(request)
	cost := max(int64(len(body)), 1)
	var stored bool
	if ttl > 0 {
		stored = c.cache.SetWithTTL(path, body, cost, ttl)
	} else {
		stored = c.cache.Set(path, body, cost)
	}
	c.cache.Wait()
	cached := false
	if stored {
		_, cached = c.cache.Get(path)
	}
	return body, cached, nil
}
