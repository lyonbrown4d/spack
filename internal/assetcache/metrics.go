package assetcache

const (
	metricAssetCacheHits           = "asset_cache_hits_total"
	metricAssetCacheMisses         = "asset_cache_misses_total"
	metricAssetCacheFills          = "asset_cache_fills_total"
	metricAssetCacheFillBytes      = "asset_cache_fill_bytes_total"
	metricAssetCacheWarmEntries    = "asset_cache_warm_entries_total"
	metricAssetCacheWarmBytes      = "asset_cache_warm_bytes_total"
	metricAssetCacheWarmupDuration = "asset_cache_warmup_duration_seconds"
	metricAssetCacheEvictions      = "asset_cache_evictions_total"
	metricAssetCacheEvictedBytes   = "asset_cache_evicted_bytes_total"
	metricAssetCacheLoadErrors     = "asset_cache_load_errors_total"
)
