package runtime

import (
	"cmp"
	"context"
	cxlist "github.com/arcgolabs/collectionx/list"
	cxset "github.com/arcgolabs/collectionx/set"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/samber/oops"
	"log/slog"
	"strings"
	"time"
)

func bootstrapCatalogOnStart(
	ctx context.Context,
	runtime catalogBootstrapRuntime,
) error {
	bootstrapErr := oops.In("runtime").Owner("catalog bootstrap")
	startedAt := time.Now()
	totalBytes, scanErr := scanCatalogAssets(ctx, runtime.scanner, runtime.cat)
	if scanErr != nil {
		return scanErr
	}
	go runtime.catMetrics.SyncCatalog(runtime.cat)
	go runtime.catMetrics.SetSourceBytes(totalBytes)

	warmErr := runtime.pipelineSvc.Warm(ctx)
	if warmErr != nil {
		return bootstrapErr.With("service", "pipeline").Wrap(warmErr)
	}
	cacheStats, cacheErr := runtime.bodyCache.Warm(ctx, runtime.cat)
	if cacheErr != nil {
		return bootstrapErr.With("service", "asset memory cache").Wrap(cacheErr)
	}
	if err := runtime.prepared.Rebuild(ctx); err != nil {
		return bootstrapErr.With("service", "prepared snapshot").Wrap(err)
	}

	runtime.logger.LogAttrs(
		ctx,
		slog.LevelInfo,
		"Catalog ready",
		catalogReadyAttrs(runtime.cfg, runtime.cat, runtime.bodyCache, cacheStats, totalBytes, time.Since(startedAt)).Values()...,
	)
	return nil
}

func scanCatalogAssets(ctx context.Context, scanner sourcecatalog.Scanner, cat catalog.Catalog) (int64, error) {
	scanErr := oops.In("runtime").Owner("catalog scan")
	snapshot, err := scanner.Scan(ctx)
	if err != nil {
		return 0, scanErr.Wrap(err)
	}

	var upsertErr error
	cxlist.NewList[*catalog.Asset](snapshot.Assets.Values()...).Sort(func(left, right *catalog.Asset) int {
		return cmp.Compare(left.Path, right.Path)
	}).Range(func(_ int, asset *catalog.Asset) bool {
		if err := cat.UpsertAsset(asset); err != nil {
			upsertErr = scanErr.With("asset_path", asset.Path).Wrap(err)
			return false
		}
		return true
	})
	if upsertErr != nil {
		return 0, upsertErr
	}

	cxlist.NewList[*catalog.Variant](snapshot.Variants.Values()...).Sort(func(left, right *catalog.Variant) int {
		return cmp.Compare(left.ID, right.ID)
	}).Range(func(_ int, variant *catalog.Variant) bool {
		if err := cat.UpsertVariant(variant); err != nil {
			upsertErr = scanErr.With("variant_id", variant.ID).With("asset_path", variant.AssetPath).Wrap(err)
			return false
		}
		return true
	})
	if upsertErr != nil {
		return 0, upsertErr
	}

	return snapshot.TotalBytes, nil
}

func logConfigOnStart(ctx context.Context, runtime catalogBootstrapRuntime) error {
	runtime.logger.LogAttrs(ctx, slog.LevelInfo, "Config loaded", configLogAttrs(runtime.cfg).Values()...)
	return nil
}

func catalogReadyAttrs(
	cfg *config.Config,
	cat catalog.Catalog,
	bodyCache *assetcache.Cache,
	cacheStats assetcache.WarmStats,
	totalBytes int64,
	duration time.Duration,
) *cxlist.List[slog.Attr] {
	attrs := cxlist.NewList(
		slog.Int("assets", cat.AssetCount()),
		slog.Int("variants", cat.VariantCount()),
		slog.Int64("bytes", totalBytes),
		slog.Bool("memory_cache_enable", bodyCache.Enabled()),
		slog.Bool("memory_cache_warmup", bodyCache.WarmupEnabled()),
		slog.Int("memory_cache_entries", cacheStats.Entries),
		slog.Int64("memory_cache_bytes", cacheStats.Bytes),
		slog.String("compression_mode", cfg.Compression.NormalizedMode()),
		slog.Duration("duration", duration),
	)
	return attrs.Merge(catalogDistributionAttrs(cat))
}

func catalogDistributionAttrs(cat catalog.Catalog) *cxlist.List[slog.Attr] {
	assetsByMediaType := cxset.NewMultiSet[string]()
	cat.AllAssets().Range(func(_ int, asset *catalog.Asset) bool {
		if asset != nil {
			addCatalogCount(assetsByMediaType, asset.MediaType)
		}
		return true
	})

	variantsByEncoding := cxset.NewMultiSet[string]()
	variantsByFormat := cxset.NewMultiSet[string]()
	cat.AllVariants().Range(func(_ int, variant *catalog.Variant) bool {
		if variant != nil {
			addCatalogCount(variantsByEncoding, variant.Encoding)
			addCatalogCount(variantsByFormat, variant.Format)
		}
		return true
	})

	return cxlist.NewList(
		slog.Any("asset_media_types", assetsByMediaType.AllCounts()),
		slog.Any("variant_encodings", variantsByEncoding.AllCounts()),
		slog.Any("variant_formats", variantsByFormat.AllCounts()),
	)
}

func addCatalogCount(counts *cxset.MultiSet[string], value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		counts.Add(value)
	}
}

func configLogAttrs(cfg *config.Config) *cxlist.List[slog.Attr] {
	return cxlist.NewList(
		slog.Int("http_port", cfg.HTTP.Port),
		slog.Bool("http_low_memory", cfg.HTTP.LowMemory),
		slog.Bool("http_memory_cache_enable", cfg.HTTP.MemoryCache.Enabled()),
		slog.Bool("http_memory_cache_warmup", cfg.HTTP.MemoryCache.WarmupEnabled()),
		slog.Int("http_memory_cache_max_entries", cfg.HTTP.MemoryCache.MaxEntries),
		slog.Int64("http_memory_cache_max_file_size", cfg.HTTP.MemoryCache.MaxFileSize),
		slog.String("http_memory_cache_ttl", cfg.HTTP.MemoryCache.ParsedTTL().String()),
		slog.String("assets_root", cfg.Assets.Root),
		slog.String("assets_path", cfg.Assets.Path),
		slog.String("assets_entry", cfg.Assets.Entry),
		slog.String("fallback_on", string(cfg.Assets.Fallback.On)),
		slog.String("fallback_target", cfg.Assets.Fallback.Target),
		slog.Int("async_workers", cfg.Async.NormalizedWorkers()),
		slog.Bool("compression_enable", cfg.Compression.Enable),
		slog.String("compression_mode", cfg.Compression.NormalizedMode()),
		slog.String("compression_cache_dir", cfg.Compression.CacheDir),
		slog.Int64("compression_min_size", cfg.Compression.MinSize),
		slog.Int("compression_workers", cfg.Compression.Workers),
		slog.Int("compression_queue_size", cfg.Compression.QueueCapacity()),
		slog.Any("compression_encodings", cfg.Compression.NormalizedEncodings().Values()),
		slog.Int("compression_zstd_level", cfg.Compression.ZstdLevel),
		slog.Bool("image_enable", cfg.Image.Enable),
		slog.Any("image_widths", cfg.Image.ParsedWidths().Values()),
		slog.Any("image_formats", cfg.Image.ParsedFormats().Values()),
		slog.Int("image_jpeg_quality", cfg.Image.JPEGQuality),
		slog.Bool("frontend_resource_hints_enable", cfg.Frontend.ResourceHints.Enable),
		slog.Bool("frontend_resource_hints_early_hints", cfg.Frontend.ResourceHints.EarlyHints),
		slog.Bool("frontend_immutable_cache_enable", cfg.Frontend.ImmutableCache.Enable),
		slog.Bool("debug_enable", cfg.Debug.Enable),
		slog.Bool("metrics_enable", cfg.Metrics.Enable),
		slog.String("metrics_prefix", cfg.Metrics.Prefix),
		slog.String("logger_level", cfg.Logger.Level),
	)
}
