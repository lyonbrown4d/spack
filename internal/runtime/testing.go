package runtime

import (
	"context"
	"log/slog"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/server"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/samber/oops"
)

func BuildCatalogAssetForTest(file source.File) (*catalog.Asset, error) {
	asset, err := sourcecatalog.BuildAsset(file)
	if err != nil {
		return nil, oops.In("runtime").Owner("testing").Wrap(err)
	}
	return asset, nil
}

func CatalogReadyAttrsForTest(
	cat catalog.Catalog,
	bodyCache *assetcache.Cache,
	cacheStats assetcache.WarmStats,
	totalBytes int64,
	duration time.Duration,
) *cxlist.List[slog.Attr] {
	return catalogReadyAttrs(cat, bodyCache, cacheStats, totalBytes, duration)
}

func MainHTTPListenConfigForTest() fiber.ListenConfig {
	return newMainHTTPListenConfig()
}

func BootstrapCatalogForTest(
	ctx context.Context,
	cfg *config.Config,
	scanner sourcecatalog.Scanner,
	cat catalog.Catalog,
	catMetrics *catalog.RuntimeMetrics,
	serverMetrics *server.RuntimeMetrics,
	bodyCache *assetcache.Cache,
	prepared *server.PreparedService,
	logger *slog.Logger,
) error {
	return bootstrapCatalogOnStart(ctx, catalogBootstrapRuntime{
		cfg:           cfg,
		scanner:       scanner,
		cat:           cat,
		catMetrics:    catMetrics,
		serverMetrics: serverMetrics,
		bodyCache:     bodyCache,
		prepared:      prepared,
		logger:        logger,
	})
}
