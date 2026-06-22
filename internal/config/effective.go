package config

import (
	"errors"
	"fmt"

	objectmapper "github.com/arcgolabs/mapper"
)

const redactedValue = "REDACTED"

type EffectiveRuntimeConfig struct {
	APIVersion  string               `yaml:"apiVersion"`
	Kind        string               `yaml:"kind"`
	HTTP        EffectiveHTTP        `yaml:"http"`
	Assets      EffectiveAssets      `yaml:"assets"`
	Async       EffectiveAsync       `yaml:"async"`
	Debug       EffectiveDebug       `yaml:"debug"`
	Image       EffectiveImage       `yaml:"image"`
	Frontend    EffectiveFrontend    `yaml:"frontend"`
	Metrics     EffectiveMetrics     `yaml:"metrics"`
	Logger      EffectiveLogger      `yaml:"logger"`
	Robots      EffectiveRobots      `yaml:"robots"`
	Compression EffectiveCompression `yaml:"compression"`
	SourceInfo  map[string]any       `yaml:"source_info,omitempty"`
}

type EffectiveHTTP struct {
	Port                int                  `yaml:"port"`
	LowMemory           bool                 `yaml:"low_memory"`
	ExposeServerHeader  bool                 `yaml:"expose_server_header"`
	ExposeServerVersion bool                 `yaml:"expose_server_version"`
	MemoryCache         EffectiveMemoryCache `yaml:"memory_cache"`
	RequestLogDetail    bool                 `yaml:"request_log_detail"`
}

type EffectiveMemoryCache struct {
	Enable      bool   `yaml:"enable"`
	Warmup      bool   `yaml:"warmup"`
	MaxEntries  int    `yaml:"max_entries"`
	MaxBytes    int64  `yaml:"max_bytes"`
	MaxFileSize int64  `yaml:"max_file_size"`
	TTL         string `yaml:"ttl"`
}

type EffectiveAssets struct {
	Path     string            `yaml:"path"`
	Root     string            `yaml:"root"`
	Entry    string            `yaml:"entry"`
	Include  []string          `yaml:"include,omitempty"`
	Exclude  []string          `yaml:"exclude,omitempty"`
	Fallback EffectiveFallback `yaml:"fallback"`
}

type EffectiveFallback struct {
	On     FallbackOn `yaml:"on"`
	Target string     `yaml:"target"`
}

type EffectiveAsync struct {
	Workers int `yaml:"workers"`
}

type EffectiveDebug struct {
	Enable      bool   `yaml:"enable"`
	PprofPrefix string `yaml:"pprof_prefix"`
}

type EffectiveImage struct {
	Enable               bool    `yaml:"enable"`
	Widths               string  `yaml:"widths"`
	Formats              string  `yaml:"formats"`
	JPEGQuality          int     `yaml:"jpeg_quality"`
	MaxSourceBytes       int64   `yaml:"max_source_bytes"`
	MaxSourcePixels      int64   `yaml:"max_source_pixels"`
	MaxWidth             int     `yaml:"max_width"`
	MaxHeight            int     `yaml:"max_height"`
	MaxOutputVariants    int     `yaml:"max_output_variants"`
	MaxConcurrentSources int     `yaml:"max_concurrent_sources"`
	MaxMemoryBytes       int64   `yaml:"max_memory_bytes"`
	MinSavingRatio       float64 `yaml:"min_saving_ratio"`
	MinSavingBytes       int64   `yaml:"min_saving_bytes"`
}

type EffectiveFrontend struct {
	ResourceHints  EffectiveResourceHints  `yaml:"resource_hints"`
	ImmutableCache EffectiveImmutableCache `yaml:"immutable_cache"`
}

type EffectiveResourceHints struct {
	Enable         bool `yaml:"enable"`
	EarlyHints     bool `yaml:"early_hints"`
	MaxLinks       int  `yaml:"max_links"`
	MaxHeaderBytes int  `yaml:"max_header_bytes"`
}

type EffectiveImmutableCache struct {
	Enable bool   `yaml:"enable"`
	MaxAge string `yaml:"max_age"`
}

type EffectiveMetrics struct {
	Enable bool   `yaml:"enable"`
	Prefix string `yaml:"prefix"`
}

type EffectiveLogger struct {
	Level   string           `yaml:"level"`
	Console EffectiveConsole `yaml:"console"`
	File    EffectiveFile    `yaml:"file"`
}

type EffectiveConsole struct {
	Enabled bool `yaml:"enabled"`
}

type EffectiveFile struct {
	Enabled  bool   `yaml:"enabled"`
	Path     string `yaml:"path"`
	MaxSize  int    `yaml:"max_size"`
	MaxAge   int    `yaml:"max_age"`
	MaxFiles int    `yaml:"max_files"`
}

type EffectiveRobots struct {
	Enable    bool   `yaml:"enable"`
	Override  bool   `yaml:"override"`
	UserAgent string `yaml:"user_agent"`
	Allow     string `yaml:"allow"`
	Disallow  string `yaml:"disallow"`
	Sitemap   string `yaml:"sitemap"`
	Host      string `yaml:"host"`
}

type EffectiveCompression struct {
	Enable                bool   `yaml:"enable"`
	Mode                  string `yaml:"mode"`
	GenerationScope       string `yaml:"generation_scope"`
	CacheDir              string `yaml:"cache_dir"`
	MinSize               int64  `yaml:"min_size"`
	Workers               int    `yaml:"workers"`
	Encodings             string `yaml:"encodings"`
	CleanupEvery          string `yaml:"cleanup_every"`
	MaxAge                string `yaml:"max_age"`
	ImageMaxAge           string `yaml:"image_max_age"`
	EncodingMaxAge        string `yaml:"encoding_max_age"`
	MaxCacheBytes         int64  `yaml:"max_cache_bytes"`
	EncodingMaxCacheBytes int64  `yaml:"encoding_max_cache_bytes"`
	ImageMaxCacheBytes    int64  `yaml:"image_max_cache_bytes"`
	BrotliQuality         int    `yaml:"brotli_quality"`
	ZstdLevel             int    `yaml:"zstd_level"`
	GzipLevel             int    `yaml:"gzip_level"`
	LazyQueueSize         *int   `yaml:"queue_size,omitempty"`
	LazyQueueSizeScope    string `yaml:"queue_size_scope,omitempty"`
}

func BuildEffectiveConfig(instance *objectmapper.Mapper, cfg *Config, redact bool) (EffectiveRuntimeConfig, error) {
	if cfg == nil {
		return EffectiveRuntimeConfig{}, nil
	}
	if instance == nil {
		return EffectiveRuntimeConfig{}, errors.New("effective config mapper is required")
	}

	var effective EffectiveRuntimeConfig
	if err := instance.MapInto(&effective, cfg, effectiveFinalizeHook(redact)); err != nil {
		return EffectiveRuntimeConfig{}, fmt.Errorf("map effective config: %w", err)
	}
	return effective, nil
}

func effectiveFinalizeHook(redact bool) objectmapper.Option {
	return objectmapper.AfterMap(func(src *Config, dst *EffectiveRuntimeConfig) {
		dst.Compression.Mode = src.Compression.NormalizedMode()
		dst.Compression.GenerationScope = compressionGenerationScope(dst.Compression.Mode)
		if dst.Compression.Mode == CompressionModeLazy {
			queueSize := src.Compression.QueueSize
			dst.Compression.LazyQueueSize = &queueSize
			dst.Compression.LazyQueueSizeScope = "legacy_runtime_enqueue_compatibility"
		} else {
			dst.Compression.LazyQueueSize = nil
			dst.Compression.LazyQueueSizeScope = ""
		}
		if redact {
			dst.redactLocalPaths()
		}
	})
}

func (cfg *EffectiveRuntimeConfig) redactLocalPaths() {
	if cfg.Assets.Root != "" {
		cfg.Assets.Root = redactedValue
	}
	if cfg.Logger.File.Path != "" {
		cfg.Logger.File.Path = redactedValue
	}
	if cfg.Compression.CacheDir != "" {
		cfg.Compression.CacheDir = redactedValue
	}
}

func compressionGenerationScope(mode string) string {
	switch mode {
	case CompressionModeWarmup:
		return "compiler_only"
	case CompressionModeLazy:
		return "legacy_runtime_enqueue_compatibility"
	default:
		return "disabled"
	}
}
