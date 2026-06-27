// Package configschema defines registry metadata for SPACK configuration flags.
package configschema

import (
	"encoding/csv"
	"strconv"
	"strings"

	"github.com/lyonbrown4d/spack/internal/config"
  "github.com/samber/lo"
  "github.com/spf13/pflag"
)

type FlagKind string

const (
	BoolFlag        FlagKind = "bool"
	Float64Flag     FlagKind = "float64"
	IntFlag         FlagKind = "int"
	Int64Flag       FlagKind = "int64"
	StringFlag      FlagKind = "string"
	StringSliceFlag FlagKind = "stringSlice"
)

type Flag struct {
	Name  string
	Kind  FlagKind
	Usage string

	boolDefault        func(config.Config) bool
	float64Default     func(config.Config) float64
	intDefault         func(config.Config) int
	int64Default       func(config.Config) int64
	stringDefault      func(config.Config) string
	stringSliceDefault func(config.Config) []string
}

func Flags() []Flag {
	return lo.Clone(configFlags)
}

func RegisterFlags(flags *pflag.FlagSet, defaults config.Config) {
	for _, flag := range configFlags {
		flag.Register(flags, defaults)
	}
}

func (f Flag) Register(flags *pflag.FlagSet, defaults config.Config) {
	switch f.Kind {
	case BoolFlag:
		flags.Bool(f.Name, f.boolDefault(defaults), f.Usage)
	case Float64Flag:
		flags.Float64(f.Name, f.float64Default(defaults), f.Usage)
	case IntFlag:
		flags.Int(f.Name, f.intDefault(defaults), f.Usage)
	case Int64Flag:
		flags.Int64(f.Name, f.int64Default(defaults), f.Usage)
	case StringFlag:
		flags.String(f.Name, f.stringDefault(defaults), f.Usage)
	case StringSliceFlag:
		flags.StringSlice(f.Name, lo.Clone(f.stringSliceDefault(defaults)), f.Usage)
	default:
		panic("unknown config flag kind: " + string(f.Kind))
	}
}

func (f Flag) DefaultString(defaults config.Config) string {
	switch f.Kind {
	case BoolFlag:
		return strconv.FormatBool(f.boolDefault(defaults))
	case Float64Flag:
		return strconv.FormatFloat(f.float64Default(defaults), 'g', -1, 64)
	case IntFlag:
		return strconv.Itoa(f.intDefault(defaults))
	case Int64Flag:
		return strconv.FormatInt(f.int64Default(defaults), 10)
	case StringFlag:
		return f.stringDefault(defaults)
	case StringSliceFlag:
		return stringSliceDefaultString(f.stringSliceDefault(defaults))
	default:
		panic("unknown config flag kind: " + string(f.Kind))
	}
}

func boolFlag(name string, defaultValue func(config.Config) bool, usage string) Flag {
	return Flag{Name: name, Kind: BoolFlag, Usage: usage, boolDefault: defaultValue}
}

func float64Flag(name string, defaultValue func(config.Config) float64, usage string) Flag {
	return Flag{Name: name, Kind: Float64Flag, Usage: usage, float64Default: defaultValue}
}

func intFlag(name string, defaultValue func(config.Config) int, usage string) Flag {
	return Flag{Name: name, Kind: IntFlag, Usage: usage, intDefault: defaultValue}
}

func int64Flag(name string, defaultValue func(config.Config) int64, usage string) Flag {
	return Flag{Name: name, Kind: Int64Flag, Usage: usage, int64Default: defaultValue}
}

func stringFlag(name string, defaultValue func(config.Config) string, usage string) Flag {
	return Flag{Name: name, Kind: StringFlag, Usage: usage, stringDefault: defaultValue}
}

func stringSliceFlag(name string, defaultValue func(config.Config) []string, usage string) Flag {
	return Flag{Name: name, Kind: StringSliceFlag, Usage: usage, stringSliceDefault: defaultValue}
}

func stringSliceDefaultString(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	var builder strings.Builder
	writer := csv.NewWriter(&builder)
	if err := writer.Write(values); err != nil {
		return "[]"
	}
	writer.Flush()
	return "[" + strings.TrimSuffix(builder.String(), "\n") + "]"
}

var configFlags = []Flag{
	intFlag("http.port", func(c config.Config) int { return c.HTTP.Port }, "HTTP listen port."),
	boolFlag("http.low_memory", func(c config.Config) bool { return c.HTTP.LowMemory }, "Reduce Fiber memory usage."),
	boolFlag("http.expose_server_header", func(c config.Config) bool { return c.HTTP.ExposeServerHeader }, "Expose the HTTP Server header with the application version."),
	boolFlag("http.expose_server_version", func(c config.Config) bool { return c.HTTP.ExposeServerVersion }, "Expose version suffix in the HTTP Server header."),
	boolFlag("http.memory_cache.enable", func(c config.Config) bool { return c.HTTP.MemoryCache.Enable }, "Enable in-memory asset cache."),
	boolFlag("http.memory_cache.warmup", func(c config.Config) bool { return c.HTTP.MemoryCache.Warmup }, "Preload in-memory asset cache at startup."),
	intFlag("http.memory_cache.max_entries", func(c config.Config) int { return c.HTTP.MemoryCache.MaxEntries }, "Expected number of in-memory asset cache entries used for admission counters."),
	int64Flag("http.memory_cache.max_bytes", func(c config.Config) int64 { return c.HTTP.MemoryCache.MaxBytes }, "Maximum total byte cost for the in-memory asset cache."),
	int64Flag("http.memory_cache.max_file_size", func(c config.Config) int64 { return c.HTTP.MemoryCache.MaxFileSize }, "Maximum asset size in bytes eligible for in-memory cache."),
	stringFlag("http.memory_cache.ttl", func(c config.Config) string { return c.HTTP.MemoryCache.TTL }, "TTL for in-memory asset cache entries."),

	stringFlag("assets.path", func(c config.Config) string { return c.Assets.Path }, "HTTP mount path for assets."),
	stringFlag("assets.root", func(c config.Config) string { return c.Assets.Root }, "Filesystem root directory or .spack bundle containing static assets."),
	stringFlag("assets.entry", func(c config.Config) string { return c.Assets.Entry }, "Default entry file for directory requests."),
	stringSliceFlag("assets.include", func(c config.Config) []string { return c.Assets.Include }, "Doublestar glob patterns included in source catalog scanning. Empty means include all files."),
	stringSliceFlag("assets.exclude", func(c config.Config) []string { return c.Assets.Exclude }, "Doublestar glob patterns excluded from source catalog scanning after includes are applied."),
	stringFlag("assets.fallback.on", func(c config.Config) string { return string(c.Assets.Fallback.On) }, "Fallback trigger mode."),
	stringFlag("assets.fallback.target", func(c config.Config) string { return c.Assets.Fallback.Target }, "Fallback asset path."),

	intFlag("async.workers", func(c config.Config) int { return c.Async.NormalizedWorkers() }, "Shared async concurrency limit."),

	boolFlag("debug.enable", func(c config.Config) bool { return c.Debug.Enable }, "Enable debug endpoints."),
	stringFlag("debug.pprof_prefix", func(c config.Config) string { return c.Debug.PprofPrefix }, "Optional prefix prepended before Fiber /debug/pprof handlers."),

	boolFlag("image.enable", func(c config.Config) bool { return c.Image.Enable }, "Enable image variant pipeline."),
	stringFlag("image.widths", func(c config.Config) string { return c.Image.Widths }, "Comma-separated responsive image widths."),
	stringFlag("image.formats", func(c config.Config) string { return c.Image.Formats }, "Comma-separated additional image output formats for warmup and default generation."),
	intFlag("image.jpeg_quality", func(c config.Config) int { return c.Image.JPEGQuality }, "JPEG encoding quality for generated variants."),
	int64Flag("image.max_source_bytes", func(c config.Config) int64 { return c.Image.MaxSourceBytes }, "Maximum source image bytes accepted by the image pipeline."),
	int64Flag("image.max_source_pixels", func(c config.Config) int64 { return c.Image.MaxSourcePixels }, "Maximum decoded source image pixels accepted by the image pipeline."),
	intFlag("image.max_width", func(c config.Config) int { return c.Image.MaxWidth }, "Maximum decoded source image width accepted by the image pipeline."),
	intFlag("image.max_height", func(c config.Config) int { return c.Image.MaxHeight }, "Maximum decoded source image height accepted by the image pipeline."),
	intFlag("image.max_output_variants", func(c config.Config) int { return c.Image.MaxOutputVariants }, "Maximum generated image variants per source asset batch."),
	intFlag("image.max_concurrent_sources", func(c config.Config) int { return c.Image.MaxConcurrentSources }, "Maximum number of source images decoded concurrently."),
	int64Flag("image.max_memory_bytes", func(c config.Config) int64 { return c.Image.MaxMemoryBytes }, "Global estimated decoded image memory budget in bytes."),
	float64Flag("image.min_saving_ratio", func(c config.Config) float64 { return c.Image.MinSavingRatio }, "Minimum source-byte saving ratio required before storing generated image variants."),
	int64Flag("image.min_saving_bytes", func(c config.Config) int64 { return c.Image.MinSavingBytes }, "Minimum saved bytes required before storing generated image variants."),

	boolFlag("frontend.resource_hints.enable", func(c config.Config) bool { return c.Frontend.ResourceHints.Enable }, "Emit Link resource hints for HTML responses."),
	boolFlag("frontend.resource_hints.early_hints", func(c config.Config) bool { return c.Frontend.ResourceHints.EarlyHints }, "Send HTTP 103 Early Hints before HTML responses."),
	intFlag("frontend.resource_hints.max_links", func(c config.Config) int { return c.Frontend.ResourceHints.MaxLinks }, "Maximum resource hint links per HTML response."),
	intFlag("frontend.resource_hints.max_header_bytes", func(c config.Config) int { return c.Frontend.ResourceHints.MaxHeaderBytes }, "Maximum Link header bytes for resource hints."),
	boolFlag("frontend.immutable_cache.enable", func(c config.Config) bool { return c.Frontend.ImmutableCache.Enable }, "Enable immutable cache headers for fingerprinted static assets."),
	stringFlag("frontend.immutable_cache.max_age", func(c config.Config) string { return c.Frontend.ImmutableCache.MaxAge }, "Cache max-age for fingerprinted static assets."),

	boolFlag("metrics.enable", func(c config.Config) bool { return c.Metrics.Enable }, "Enable Prometheus metrics endpoint and runtime collectors."),
	stringFlag("metrics.prefix", func(c config.Config) string { return c.Metrics.Prefix }, "Metrics endpoint path."),

	stringFlag("logger.level", func(c config.Config) string { return c.Logger.Level }, "Logger level."),
	boolFlag("logger.console.enabled", func(c config.Config) bool { return c.Logger.Console.Enabled }, "Enable console logging."),
	boolFlag("logger.file.enabled", func(c config.Config) bool { return c.Logger.File.Enabled }, "Enable file logging."),
	stringFlag("logger.file.path", func(c config.Config) string { return c.Logger.File.Path }, "Log file path."),
	intFlag("logger.file.max_size", func(c config.Config) int { return c.Logger.File.MaxSize }, "Maximum log file size before rotation."),
	intFlag("logger.file.max_age", func(c config.Config) int { return c.Logger.File.MaxAge }, "Maximum age in days for rotated log files."),
	intFlag("logger.file.max_files", func(c config.Config) int { return c.Logger.File.MaxFiles }, "Maximum number of rotated log files to retain."),

	boolFlag("robots.enable", func(c config.Config) bool { return c.Robots.Enable }, "Enable built-in robots.txt route generation."),
	boolFlag("robots.override", func(c config.Config) bool { return c.Robots.Override }, "Prefer generated robots.txt over a scanned robots.txt asset."),
	stringFlag("robots.user_agent", func(c config.Config) string { return c.Robots.UserAgent }, "Generated robots.txt User-agent value."),
	stringFlag("robots.allow", func(c config.Config) string { return c.Robots.Allow }, "Generated robots.txt Allow value."),
	stringFlag("robots.disallow", func(c config.Config) string { return c.Robots.Disallow }, "Generated robots.txt Disallow value."),
	stringFlag("robots.sitemap", func(c config.Config) string { return c.Robots.Sitemap }, "Generated robots.txt Sitemap value."),
	stringFlag("robots.host", func(c config.Config) string { return c.Robots.Host }, "Generated robots.txt Host value."),

	boolFlag("compression.enable", func(c config.Config) bool { return c.Compression.Enable }, "Enable compression pipeline."),
	stringFlag("compression.mode", func(c config.Config) string { return c.Compression.Mode }, "Compression mode: off, lazy, or warmup."),
	stringFlag("compression.cache_dir", func(c config.Config) string { return c.Compression.CacheDir }, "Compression artifact cache directory."),
	int64Flag("compression.min_size", func(c config.Config) int64 { return c.Compression.MinSize }, "Minimum asset size in bytes eligible for compression."),
	intFlag("compression.workers", func(c config.Config) int { return c.Compression.Workers }, "Compression worker count."),
	intFlag("compression.queue_size", func(c config.Config) int { return c.Compression.QueueSize }, "Compression queue capacity."),
	stringFlag("compression.encodings", func(c config.Config) string { return c.Compression.Encodings }, "Comma-separated supported compression encodings in preference order."),
	stringFlag("compression.cleanup_every", func(c config.Config) string { return c.Compression.CleanupEvery }, "Compression cache cleanup interval."),
	stringFlag("compression.max_age", func(c config.Config) string { return c.Compression.MaxAge }, "Default cache max-age for compressed responses."),
	stringFlag("compression.image_max_age", func(c config.Config) string { return c.Compression.ImageMaxAge }, "Cache max-age for generated image variants."),
	stringFlag("compression.encoding_max_age", func(c config.Config) string { return c.Compression.EncodingMaxAge }, "Cache max-age for precompressed variants."),
	int64Flag("compression.max_cache_bytes", func(c config.Config) int64 { return c.Compression.MaxCacheBytes }, "Maximum bytes allowed in compression cache."),
	int64Flag("compression.encoding_max_cache_bytes", func(c config.Config) int64 { return c.Compression.EncodingMaxCacheBytes }, "Maximum bytes allowed for precompressed artifacts."),
	int64Flag("compression.image_max_cache_bytes", func(c config.Config) int64 { return c.Compression.ImageMaxCacheBytes }, "Maximum bytes allowed for generated image artifacts."),
	intFlag("compression.brotli_quality", func(c config.Config) int { return c.Compression.BrotliQuality }, "Brotli compression quality."),
	intFlag("compression.zstd_level", func(c config.Config) int { return c.Compression.ZstdLevel }, "Zstd compression level."),
	intFlag("compression.gzip_level", func(c config.Config) int { return c.Compression.GzipLevel }, "Gzip compression level."),
}
