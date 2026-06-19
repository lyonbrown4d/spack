package runtime

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/samber/oops"
	"log/slog"
	"time"
)

func BuildCatalogAssetForTest(file source.File) (*catalog.Asset, error) {
	asset, err := sourcecatalog.BuildAsset(file)
	if err != nil {
		return nil, oops.In("runtime").Owner("testing").Wrap(err)
	}
	return asset, nil
}

func CatalogReadyAttrsForTest(
	cfg *config.Config,
	cat catalog.Catalog,
	bodyCache *assetcache.Cache,
	cacheStats assetcache.WarmStats,
	totalBytes int64,
	duration time.Duration,
) *cxlist.List[slog.Attr] {
	return catalogReadyAttrs(cfg, cat, bodyCache, cacheStats, totalBytes, duration)
}

func MainHTTPListenConfigForTest(cfg *config.Config) fiber.ListenConfig {
	return newMainHTTPListenConfig(cfg)
}
