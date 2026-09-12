package config

import (
	"strings"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	contentcodingspec "github.com/lyonbrown4d/spack/internal/contentcoding/spec"
	"github.com/lyonbrown4d/spack/internal/validation"
)

const (
	CompressionModeOff    = "off"
	CompressionModeLazy   = "lazy"
	CompressionModeWarmup = "warmup"
)

type Compression struct {
	Enable                bool   `configx:"usage=Enable compression pipeline."                                         koanf:"enable"`
	Mode                  string `configx:"usage=Compression mode: off lazy or warmup."                                koanf:"mode"                     validate:"required,oneof=off lazy warmup"`
	CacheDir              string `configx:"usage=Compression artifact cache directory."                                koanf:"cache_dir"                validate:"required"`
	MinSize               int64  `configx:"usage=Minimum asset size in bytes eligible for compression."                koanf:"min_size"                 validate:"gte=0"`
	Workers               int    `configx:"usage=Compression worker count."                                            koanf:"workers"                  validate:"gte=0"`
	QueueSize             int    `configx:"usage=Compression queue capacity."                                          koanf:"queue_size"               validate:"gte=0"`
	Encodings             string `configx:"usage=Comma-separated supported compression encodings in preference order." koanf:"encodings"`
	CleanupEvery          string `configx:"usage=Compression cache cleanup interval."                                  koanf:"cleanup_every"            validate:"omitempty,spack_duration"`
	MaxAge                string `configx:"usage=Default cache max-age for compressed responses."                      koanf:"max_age"                  validate:"omitempty,spack_flexible_duration"`
	ImageMaxAge           string `configx:"usage=Cache max-age for generated image variants."                          koanf:"image_max_age"            validate:"omitempty,spack_flexible_duration"`
	EncodingMaxAge        string `configx:"usage=Cache max-age for precompressed variants."                            koanf:"encoding_max_age"         validate:"omitempty,spack_flexible_duration"`
	MaxCacheBytes         int64  `configx:"usage=Maximum bytes allowed in compression cache."                          koanf:"max_cache_bytes"          validate:"gte=0"`
	EncodingMaxCacheBytes int64  `configx:"usage=Maximum bytes allowed for precompressed artifacts."                   koanf:"encoding_max_cache_bytes" validate:"gte=0"`
	ImageMaxCacheBytes    int64  `configx:"usage=Maximum bytes allowed for generated image artifacts."                 koanf:"image_max_cache_bytes"    validate:"gte=0"`
	BrotliQuality         int    `configx:"usage=Brotli compression quality."                                          koanf:"brotli_quality"           validate:"gte=0,lte=11"`
	ZstdLevel             int    `configx:"usage=Zstd compression level."                                              koanf:"zstd_level"               validate:"gte=0,lte=22"`
	GzipLevel             int    `configx:"usage=Gzip compression level."                                              koanf:"gzip_level"               validate:"gte=-2,lte=9"`
}

func (c Compression) NormalizedMode() string {
	switch strings.ToLower(strings.TrimSpace(c.Mode)) {
	case "", CompressionModeWarmup:
		return CompressionModeWarmup
	case CompressionModeOff:
		return CompressionModeOff
	case CompressionModeLazy:
		return CompressionModeLazy
	default:
		return CompressionModeWarmup
	}
}

func (c Compression) PipelineEnabled() bool {
	return c.Enable && c.NormalizedMode() != CompressionModeOff
}

func (c Compression) QueueCapacity() int {
	if c.QueueSize > 0 {
		return c.QueueSize
	}
	workers := max(c.Workers, 1)
	return workers * 64
}

func (c Compression) NormalizedEncodings() *cxlist.List[string] {
	return contentcodingspec.ResolveNames(c.Encodings)
}

func (c Compression) ParsedCleanupInterval() time.Duration {
	raw := strings.TrimSpace(c.CleanupEvery)
	if raw == "" {
		return 5 * time.Minute
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 5 * time.Minute
	}
	return d
}

func (c Compression) ParsedMaxAge() time.Duration {
	return validation.ParseFlexibleDuration(c.MaxAge)
}

func (c Compression) NamespaceMaxAges() *cxmapping.Map[string, time.Duration] {
	out := cxmapping.NewMapWithCapacity[string, time.Duration](2)
	if d := validation.ParseFlexibleDuration(c.EncodingMaxAge); d > 0 {
		out.Set("encoding", d)
	}
	if d := validation.ParseFlexibleDuration(c.ImageMaxAge); d > 0 {
		out.Set("image", d)
	}
	return out
}

func (c Compression) NamespaceMaxCacheBytes() *cxmapping.Map[string, int64] {
	out := cxmapping.NewMapWithCapacity[string, int64](2)
	if c.EncodingMaxCacheBytes > 0 {
		out.Set("encoding", c.EncodingMaxCacheBytes)
	}
	if c.ImageMaxCacheBytes > 0 {
		out.Set("image", c.ImageMaxCacheBytes)
	}
	return out
}
