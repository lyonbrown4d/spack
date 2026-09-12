package assetcache

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/source"
)

// NewCacheForTest exposes cache construction for external tests.
func NewCacheForTest(cfg config.MemoryCache, logger *slog.Logger) *Cache {
	return NewCacheWithObservabilityForTest(cfg, logger, nil)
}

// NewCacheWithRootForTest exposes cache construction bound to a specific local source root.
func NewCacheWithRootForTest(cfg config.MemoryCache, logger *slog.Logger, root string) *Cache {
	testCfg := newCacheConfigForTest(cfg)
	testCfg.Assets.Root = root
	cache, err := newCache(testCfg, logger, nil, nil, nil, nil)
	if err != nil {
		return nil
	}
	return cache
}

// NewCacheWithSourceForTest exposes cache construction with an injected local source.
func NewCacheWithSourceForTest(cfg config.MemoryCache, logger *slog.Logger, src *source.LocalFS) *Cache {
	testCfg := newCacheConfigForTest(cfg)
	cache, err := newCache(testCfg, logger, nil, nil, nil, src)
	if err != nil {
		return nil
	}
	return cache
}

// NewCacheWithObservabilityForTest exposes cache construction with observability for external tests.
func NewCacheWithObservabilityForTest(
	cfg config.MemoryCache,
	logger *slog.Logger,
	obs observabilityx.Observability,
) *Cache {
	testCfg := newCacheConfigForTest(cfg)
	cache, err := newCache(testCfg, logger, obs, nil, nil, nil)
	if err != nil {
		return nil
	}
	return cache
}

// NewCacheWithBusForTest exposes cache construction with an event bus for external tests.
func NewCacheWithBusForTest(
	cfg config.MemoryCache,
	logger *slog.Logger,
	obs observabilityx.Observability,
	bus *eventx.Bus,
) *Cache {
	testCfg := newCacheConfigForTest(cfg)
	cache, err := newCache(testCfg, logger, obs, bus, nil, nil)
	if err != nil {
		return nil
	}
	return cache
}

// NewCacheWithSettingsForTest exposes cache construction with shared worker settings for external tests.
func NewCacheWithSettingsForTest(
	cfg config.MemoryCache,
	logger *slog.Logger,
	obs observabilityx.Observability,
	settings *asyncx.Settings,
) *Cache {
	testCfg := newCacheConfigForTest(cfg)
	cache, err := newCache(testCfg, logger, obs, nil, settings, nil)
	if err != nil {
		return nil
	}
	return cache
}

func newCacheConfigForTest(cfg config.MemoryCache) *config.Config {
	testCfg := config.DefaultConfigForTest()
	testCfg.HTTP.MemoryCache = cfg
	testCfg.Assets.Root = testCacheRootForTest()
	return &testCfg
}

func testCacheRootForTest() string {
	for _, dir := range []string{os.Getenv("GOTMPDIR"), os.TempDir()} {
		if dir == "" {
			continue
		}
		absolute, err := filepath.Abs(filepath.Clean(dir))
		if err == nil {
			return absolute
		}
	}
	return string(os.PathSeparator)
}

// StartForTest exposes cache lifecycle start for external tests.
func StartForTest(cache *Cache) error {
	return cache.start(context.TODO())
}

// StopForTest exposes cache lifecycle stop for external tests.
func StopForTest(cache *Cache) error {
	return cache.stop(context.TODO())
}

// FileSourceForTest exposes the cache file source and its ownership state.
func FileSourceForTest(cache *Cache) (*source.LocalFS, bool) {
	if cache == nil {
		return nil, false
	}
	return cache.files, cache.filesOwned
}
