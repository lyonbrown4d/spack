package config

const redactedValue = "REDACTED"

func EffectiveMap(cfg *Config, redact bool) map[string]any {
	if cfg == nil {
		return map[string]any{}
	}
	return map[string]any{
		"apiVersion": cfg.APIVersion,
		"kind":       cfg.Kind,
		"http":       effectiveHTTPMap(cfg.HTTP),
		"assets":     effectiveAssetsMap(cfg.Assets, redact),
		"async": map[string]any{
			"workers": cfg.Async.Workers,
		},
		"debug": map[string]any{
			"enable":       cfg.Debug.Enable,
			"pprof_prefix": cfg.Debug.PprofPrefix,
		},
		"image": map[string]any{
			"enable":                 cfg.Image.Enable,
			"widths":                 cfg.Image.Widths,
			"formats":                cfg.Image.Formats,
			"jpeg_quality":           cfg.Image.JPEGQuality,
			"max_source_bytes":       cfg.Image.MaxSourceBytes,
			"max_source_pixels":      cfg.Image.MaxSourcePixels,
			"max_width":              cfg.Image.MaxWidth,
			"max_height":             cfg.Image.MaxHeight,
			"max_output_variants":    cfg.Image.MaxOutputVariants,
			"max_concurrent_sources": cfg.Image.MaxConcurrentSources,
			"max_memory_bytes":       cfg.Image.MaxMemoryBytes,
			"min_saving_ratio":       cfg.Image.MinSavingRatio,
			"min_saving_bytes":       cfg.Image.MinSavingBytes,
		},
		"frontend":    effectiveFrontendMap(cfg.Frontend),
		"metrics":     effectiveMetricsMap(cfg.Metrics),
		"logger":      effectiveLoggerMap(cfg.Logger, redact),
		"robots":      effectiveRobotsMap(cfg.Robots),
		"compression": effectiveCompressionMap(cfg.Compression, redact),
	}
}

func effectiveHTTPMap(cfg HTTP) map[string]any {
	return map[string]any{
		"port":                  cfg.Port,
		"low_memory":            cfg.LowMemory,
		"expose_server_header":  cfg.ExposeServerHeader,
		"expose_server_version": cfg.ExposeServerVersion,
		"request_log_detail":    cfg.RequestLogDetail,
		"memory_cache": map[string]any{
			"enable":        cfg.MemoryCache.Enable,
			"warmup":        cfg.MemoryCache.Warmup,
			"max_entries":   cfg.MemoryCache.MaxEntries,
			"max_bytes":     cfg.MemoryCache.MaxBytes,
			"max_file_size": cfg.MemoryCache.MaxFileSize,
			"ttl":           cfg.MemoryCache.TTL,
		},
	}
}

func effectiveAssetsMap(cfg Assets, redact bool) map[string]any {
	root := cfg.Root
	if redact && root != "" {
		root = redactedValue
	}
	return map[string]any{
		"path":  cfg.Path,
		"root":  root,
		"entry": cfg.Entry,
		"fallback": map[string]any{
			"on":     cfg.Fallback.On,
			"target": cfg.Fallback.Target,
		},
	}
}

func effectiveFrontendMap(cfg Frontend) map[string]any {
	return map[string]any{
		"resource_hints": map[string]any{
			"enable":           cfg.ResourceHints.Enable,
			"early_hints":      cfg.ResourceHints.EarlyHints,
			"max_links":        cfg.ResourceHints.MaxLinks,
			"max_header_bytes": cfg.ResourceHints.MaxHeaderBytes,
		},
		"immutable_cache": map[string]any{
			"enable":  cfg.ImmutableCache.Enable,
			"max_age": cfg.ImmutableCache.MaxAge,
		},
	}
}

func effectiveMetricsMap(cfg Metrics) map[string]any {
	return map[string]any{
		"enable": cfg.Enable,
		"prefix": cfg.Prefix,
	}
}

func effectiveLoggerMap(cfg Logger, redact bool) map[string]any {
	path := cfg.File.Path
	if redact && path != "" {
		path = redactedValue
	}
	return map[string]any{
		"level": cfg.Level,
		"console": map[string]any{
			"enabled": cfg.Console.Enabled,
		},
		"file": map[string]any{
			"enabled":   cfg.File.Enabled,
			"path":      path,
			"max_size":  cfg.File.MaxSize,
			"max_age":   cfg.File.MaxAge,
			"max_files": cfg.File.MaxFiles,
		},
	}
}

func effectiveRobotsMap(cfg Robots) map[string]any {
	return map[string]any{
		"enable":     cfg.Enable,
		"override":   cfg.Override,
		"user_agent": cfg.UserAgent,
		"allow":      cfg.Allow,
		"disallow":   cfg.Disallow,
		"sitemap":    cfg.Sitemap,
		"host":       cfg.Host,
	}
}

func effectiveCompressionMap(cfg Compression, redact bool) map[string]any {
	cacheDir := cfg.CacheDir
	if redact && cacheDir != "" {
		cacheDir = redactedValue
	}
	return map[string]any{
		"enable":                   cfg.Enable,
		"mode":                     cfg.Mode,
		"cache_dir":                cacheDir,
		"min_size":                 cfg.MinSize,
		"workers":                  cfg.Workers,
		"queue_size":               cfg.QueueSize,
		"encodings":                cfg.Encodings,
		"cleanup_every":            cfg.CleanupEvery,
		"max_age":                  cfg.MaxAge,
		"image_max_age":            cfg.ImageMaxAge,
		"encoding_max_age":         cfg.EncodingMaxAge,
		"max_cache_bytes":          cfg.MaxCacheBytes,
		"encoding_max_cache_bytes": cfg.EncodingMaxCacheBytes,
		"image_max_cache_bytes":    cfg.ImageMaxCacheBytes,
		"brotli_quality":           cfg.BrotliQuality,
		"zstd_level":               cfg.ZstdLevel,
		"gzip_level":               cfg.GzipLevel,
	}
}
