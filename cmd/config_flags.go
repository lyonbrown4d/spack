package cmd

import (
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	configFiles   []string
	configFlagSet = newConfigFlagSet()
)

func init() {
	bindConfigFlags(rootCmd)
}

func bindConfigFlags(cmd *cobra.Command) {
	flags := cmd.PersistentFlags()
	flags.StringSliceVarP(&configFiles, "config", "c", nil, "Config file path(s). Later files override earlier ones.")
	flags.AddFlagSet(configFlagSet)
}

func configLoadOptions(cmd *cobra.Command) config.LoadOptions {
	return config.LoadOptions{
		Files:   append([]string(nil), configFiles...),
		FlagSet: cmd.Flags(),
	}
}

func newConfigFlagSet() *pflag.FlagSet {
	defaults := config.DefaultConfig()
	flags := pflag.NewFlagSet("config", pflag.ContinueOnError)

	bindHTTPFlags(flags, defaults.HTTP)
	bindAssetFlags(flags, defaults.Assets)
	bindAsyncFlags(flags, defaults.Async)
	bindDebugFlags(flags, defaults.Debug)
	bindImageFlags(flags, defaults.Image)
	bindFrontendFlags(flags, defaults.Frontend)
	bindMetricsFlags(flags, defaults.Metrics)
	bindLoggerFlags(flags, defaults.Logger)
	bindRobotsFlags(flags, defaults.Robots)
	bindCompressionFlags(flags, defaults.Compression)

	return flags
}

func bindHTTPFlags(flags *pflag.FlagSet, defaults config.HTTP) {
	flags.Int("http.port", defaults.Port, "HTTP listen port.")
	flags.Bool("http.low_memory", defaults.LowMemory, "Reduce Fiber memory usage.")
	flags.Bool("http.expose_server_header", defaults.ExposeServerHeader, "Expose the HTTP Server header with the application version.")
	flags.Bool("http.expose_server_version", defaults.ExposeServerVersion, "Expose version suffix in the HTTP Server header.")
	flags.Bool("http.memory_cache.enable", defaults.MemoryCache.Enable, "Enable in-memory asset cache.")
	flags.Bool("http.memory_cache.warmup", defaults.MemoryCache.Warmup, "Preload in-memory asset cache at startup.")
	flags.Int("http.memory_cache.max_entries", defaults.MemoryCache.MaxEntries, "Expected number of in-memory asset cache entries used for admission counters.")
	flags.Int64("http.memory_cache.max_bytes", defaults.MemoryCache.MaxBytes, "Maximum total byte cost for the in-memory asset cache.")
	flags.Int64("http.memory_cache.max_file_size", defaults.MemoryCache.MaxFileSize, "Maximum asset size in bytes eligible for in-memory cache.")
	flags.String("http.memory_cache.ttl", defaults.MemoryCache.TTL, "TTL for in-memory asset cache entries.")
}

func bindAssetFlags(flags *pflag.FlagSet, defaults config.Assets) {
	flags.String("assets.path", defaults.Path, "HTTP mount path for assets.")
	flags.String("assets.root", defaults.Root, "Filesystem root containing static assets.")
	flags.String("assets.entry", defaults.Entry, "Default entry file for directory requests.")
	flags.String("assets.fallback.on", string(defaults.Fallback.On), "Fallback trigger mode.")
	flags.String("assets.fallback.target", defaults.Fallback.Target, "Fallback asset path.")
}

func bindAsyncFlags(flags *pflag.FlagSet, defaults config.Async) {
	flags.Int("async.workers", defaults.NormalizedWorkers(), "Shared async concurrency limit.")
}

func bindDebugFlags(flags *pflag.FlagSet, defaults config.Debug) {
	flags.Bool("debug.enable", defaults.Enable, "Enable debug endpoints.")
	flags.String("debug.pprof_prefix", defaults.PprofPrefix, "Optional prefix prepended before Fiber /debug/pprof handlers.")
}

func bindImageFlags(flags *pflag.FlagSet, defaults config.Image) {
	flags.Bool("image.enable", defaults.Enable, "Enable image variant pipeline.")
	flags.String("image.widths", defaults.Widths, "Comma-separated responsive image widths.")
	flags.String("image.formats", defaults.Formats, "Comma-separated additional image output formats for warmup and default generation.")
	flags.Int("image.jpeg_quality", defaults.JPEGQuality, "JPEG encoding quality for generated variants.")
	flags.Int64("image.max_source_bytes", defaults.MaxSourceBytes, "Maximum source image bytes accepted by the image pipeline.")
	flags.Int64("image.max_source_pixels", defaults.MaxSourcePixels, "Maximum decoded source image pixels accepted by the image pipeline.")
	flags.Int("image.max_width", defaults.MaxWidth, "Maximum decoded source image width accepted by the image pipeline.")
	flags.Int("image.max_height", defaults.MaxHeight, "Maximum decoded source image height accepted by the image pipeline.")
	flags.Int("image.max_output_variants", defaults.MaxOutputVariants, "Maximum generated image variants per source asset batch.")
	flags.Int("image.max_concurrent_sources", defaults.MaxConcurrentSources, "Maximum number of source images decoded concurrently.")
	flags.Int64("image.max_memory_bytes", defaults.MaxMemoryBytes, "Global estimated decoded image memory budget in bytes.")
	flags.Float64("image.min_saving_ratio", defaults.MinSavingRatio, "Minimum source-byte saving ratio required before storing generated image variants.")
	flags.Int64("image.min_saving_bytes", defaults.MinSavingBytes, "Minimum saved bytes required before storing generated image variants.")
}

func bindFrontendFlags(flags *pflag.FlagSet, defaults config.Frontend) {
	flags.Bool("frontend.resource_hints.enable", defaults.ResourceHints.Enable, "Emit Link resource hints for HTML responses.")
	flags.Bool("frontend.resource_hints.early_hints", defaults.ResourceHints.EarlyHints, "Send HTTP 103 Early Hints before HTML responses.")
	flags.Int("frontend.resource_hints.max_links", defaults.ResourceHints.MaxLinks, "Maximum resource hint links per HTML response.")
	flags.Int("frontend.resource_hints.max_header_bytes", defaults.ResourceHints.MaxHeaderBytes, "Maximum Link header bytes for resource hints.")
	flags.Bool("frontend.immutable_cache.enable", defaults.ImmutableCache.Enable, "Enable immutable cache headers for fingerprinted static assets.")
	flags.String("frontend.immutable_cache.max_age", defaults.ImmutableCache.MaxAge, "Cache max-age for fingerprinted static assets.")
}

func bindMetricsFlags(flags *pflag.FlagSet, defaults config.Metrics) {
	flags.Bool("metrics.enable", defaults.Enable, "Enable Prometheus metrics endpoint and runtime collectors.")
	flags.String("metrics.prefix", defaults.Prefix, "Metrics endpoint path.")
}

func bindLoggerFlags(flags *pflag.FlagSet, defaults config.Logger) {
	flags.String("logger.level", defaults.Level, "Logger level.")
	flags.Bool("logger.console.enabled", defaults.Console.Enabled, "Enable console logging.")
	flags.Bool("logger.file.enabled", defaults.File.Enabled, "Enable file logging.")
	flags.String("logger.file.path", defaults.File.Path, "Log file path.")
	flags.Int("logger.file.max_size", defaults.File.MaxSize, "Maximum log file size before rotation.")
	flags.Int("logger.file.max_age", defaults.File.MaxAge, "Maximum age in days for rotated log files.")
	flags.Int("logger.file.max_files", defaults.File.MaxFiles, "Maximum number of rotated log files to retain.")
}

func bindRobotsFlags(flags *pflag.FlagSet, defaults config.Robots) {
	flags.Bool("robots.enable", defaults.Enable, "Enable built-in robots.txt route generation.")
	flags.Bool("robots.override", defaults.Override, "Prefer generated robots.txt over a scanned robots.txt asset.")
	flags.String("robots.user_agent", defaults.UserAgent, "Generated robots.txt User-agent value.")
	flags.String("robots.allow", defaults.Allow, "Generated robots.txt Allow value.")
	flags.String("robots.disallow", defaults.Disallow, "Generated robots.txt Disallow value.")
	flags.String("robots.sitemap", defaults.Sitemap, "Generated robots.txt Sitemap value.")
	flags.String("robots.host", defaults.Host, "Generated robots.txt Host value.")
}

func bindCompressionFlags(flags *pflag.FlagSet, defaults config.Compression) {
	flags.Bool("compression.enable", defaults.Enable, "Enable compression pipeline.")
	flags.String("compression.mode", defaults.Mode, "Compression mode: off, lazy, or warmup.")
	flags.String("compression.cache_dir", defaults.CacheDir, "Compression artifact cache directory.")
	flags.Int64("compression.min_size", defaults.MinSize, "Minimum asset size in bytes eligible for compression.")
	flags.Int("compression.workers", defaults.Workers, "Compression worker count.")
	flags.Int("compression.queue_size", defaults.QueueSize, "Compression queue capacity.")
	flags.String("compression.encodings", defaults.Encodings, "Comma-separated supported compression encodings in preference order.")
	flags.String("compression.cleanup_every", defaults.CleanupEvery, "Compression cache cleanup interval.")
	flags.String("compression.max_age", defaults.MaxAge, "Default cache max-age for compressed responses.")
	flags.String("compression.image_max_age", defaults.ImageMaxAge, "Cache max-age for generated image variants.")
	flags.String("compression.encoding_max_age", defaults.EncodingMaxAge, "Cache max-age for precompressed variants.")
	flags.Int64("compression.max_cache_bytes", defaults.MaxCacheBytes, "Maximum bytes allowed in compression cache.")
	flags.Int64("compression.encoding_max_cache_bytes", defaults.EncodingMaxCacheBytes, "Maximum bytes allowed for precompressed artifacts.")
	flags.Int64("compression.image_max_cache_bytes", defaults.ImageMaxCacheBytes, "Maximum bytes allowed for generated image artifacts.")
	flags.Int("compression.brotli_quality", defaults.BrotliQuality, "Brotli compression quality.")
	flags.Int("compression.zstd_level", defaults.ZstdLevel, "Zstd compression level.")
	flags.Int("compression.gzip_level", defaults.GzipLevel, "Gzip compression level.")
}
