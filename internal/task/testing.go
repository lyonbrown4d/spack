package task

import (
	"context"
	"time"

	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/artifact"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
)

// SyncSourceCatalogForTest exposes source/catalog reconciliation for black-box tests.
func SyncSourceCatalogForTest(
	ctx context.Context,
	src *source.LocalFS,
	cat catalog.Catalog,
	bodyCache *assetcache.Cache,
	changes ...source.ChangeEvent,
) (SourceRescanReport, error) {
	cfg := config.DefaultConfigForTest()
	scanner := sourcecatalog.NewScanner(src, contentcoding.NewRegistry(contentcoding.Options{
		BrotliQuality: cfg.Compression.BrotliQuality,
		GzipLevel:     cfg.Compression.GzipLevel,
		ZstdLevel:     cfg.Compression.ZstdLevel,
	}, cfg.Compression.NormalizedEncodings()))
	return syncSourceCatalog(ctx, scanner, cat, bodyCache, changes...)
}

// SyncArtifactCatalogForTest exposes artifact/catalog reconciliation for black-box tests.
func SyncArtifactCatalogForTest(
	ctx context.Context,
	store artifact.Store,
	cat catalog.Catalog,
	bodyCache *assetcache.Cache,
) (ArtifactJanitorReport, error) {
	return syncArtifactCatalog(ctx, store, cat, bodyCache)
}

// WarmCacheHotsetForTest exposes hotset warming for black-box tests.
func WarmCacheHotsetForTest(
	ctx context.Context,
	cfg *config.Config,
	cat catalog.Catalog,
	bodyCache *assetcache.Cache,
) (CacheWarmerReport, error) {
	return warmCacheHotset(ctx, cfg, cat, bodyCache)
}

// RecordTaskRunMetricsForTest exposes task run metric recording for external tests.
func RecordTaskRunMetricsForTest(
	ctx context.Context,
	obs observabilityx.Observability,
	taskName string,
	startedAt time.Time,
	err error,
) {
	recordTaskRunMetrics(ctx, obs, taskName, startedAt, err)
}

// RecordSourceRescanMetricsForTest exposes source-rescan metric recording for external tests.
func RecordSourceRescanMetricsForTest(ctx context.Context, obs observabilityx.Observability, report SourceRescanReport) {
	recordSourceRescanMetrics(ctx, obs, report)
}

// RecordArtifactJanitorMetricsForTest exposes janitor metric recording for external tests.
func RecordArtifactJanitorMetricsForTest(ctx context.Context, obs observabilityx.Observability, report ArtifactJanitorReport) {
	recordArtifactJanitorMetrics(ctx, obs, report)
}

// RecordCacheWarmerMetricsForTest exposes cache-warmer metric recording for external tests.
func RecordCacheWarmerMetricsForTest(ctx context.Context, obs observabilityx.Observability, report CacheWarmerReport) {
	recordCacheWarmerMetrics(ctx, obs, report)
}
