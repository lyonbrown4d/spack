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
)

// NewCacheForTest exposes cache construction for external tests.
func NewCacheForTest(cfg config.MemoryCache, logger *slog.Logger) *Cache {
	return NewCacheWithObservabilityForTest(cfg, logger, nil)
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
		panic(err)
	}
	return cache
}

// NewCacheWithBusForTest exposes cache construction with an event bus for external tests.
func NewCacheWithBusForTest(
	cfg config.MemoryCache,
	logger *slog.Logger,
	obs observabilityx.Observability,
	bus eventx.BusRuntime,
) *Cache {
	testCfg := newCacheConfigForTest(cfg)
	cache, err := newCache(testCfg, logger, obs, bus, nil, nil)
	if err != nil {
		panic(err)
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
		panic(err)
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
		volume := filepath.VolumeName(dir)
		if volume == "" {
			return string(os.PathSeparator)
		}
		return volume + string(os.PathSeparator)
	}
	return string(os.PathSeparator)
}

// StartForTest exposes cache lifecycle start for external tests.
func StartForTest(cache *Cache) error {
	return cache.start(context.Background())
}
