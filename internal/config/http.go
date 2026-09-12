package config

import (
	"strconv"
	"strings"
	"time"
)

type HTTP struct {
	Port                int         `configx:"usage=HTTP listen port."                                           koanf:"port"                  validate:"gte=1,lte=65535"`
	LowMemory           bool        `configx:"usage=Reduce Fiber memory usage."                                  koanf:"low_memory"`
	ExposeServerHeader  bool        `configx:"usage=Expose the HTTP Server header with the application version." koanf:"expose_server_header"`
	ExposeServerVersion bool        `configx:"usage=Expose version suffix in the HTTP Server header."            koanf:"expose_server_version"`
	MemoryCache         MemoryCache `koanf:"memory_cache"                                                        validate:"required"`
	RequestLogDetail    bool        `configx:"nocli"                                                             koanf:"request_log_detail"`
}

type MemoryCache struct {
	Enable      bool   `configx:"usage=Enable in-memory asset cache."                                                 koanf:"enable"`
	Warmup      bool   `configx:"usage=Preload in-memory asset cache at startup."                                     koanf:"warmup"`
	MaxEntries  int    `configx:"usage=Expected number of in-memory asset cache entries used for admission counters." koanf:"max_entries"   validate:"gte=0"`
	MaxBytes    int64  `configx:"usage=Maximum total byte cost for the in-memory asset cache."                        koanf:"max_bytes"     validate:"gte=0"`
	MaxFileSize int64  `configx:"usage=Maximum asset size in bytes eligible for in-memory cache."                     koanf:"max_file_size" validate:"gte=0"`
	TTL         string `configx:"usage=TTL for in-memory asset cache entries."                                        koanf:"ttl"           validate:"omitempty,spack_duration"`
}

func (h HTTP) GetPort() string {
	return strconv.Itoa(h.Port)
}

func (c MemoryCache) Enabled() bool {
	return c.Enable && c.MaxEntries > 0 && c.MaxCost() > 0 && c.MaxFileSize > 0 && c.ParsedTTL() > 0
}

func (c MemoryCache) WarmupEnabled() bool {
	return c.Enabled() && c.Warmup
}

func (c MemoryCache) ParsedTTL() time.Duration {
	raw := strings.TrimSpace(c.TTL)
	if raw == "" {
		return 5 * time.Minute
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 5 * time.Minute
	}
	return d
}

func (c MemoryCache) MaxCost() int64 {
	if c.MaxBytes > 0 {
		return c.MaxBytes
	}
	if c.MaxEntries <= 0 || c.MaxFileSize <= 0 {
		return 0
	}
	entries := int64(c.MaxEntries)
	if entries > (1<<63-1)/c.MaxFileSize {
		return 1<<63 - 1
	}
	return entries * c.MaxFileSize
}

func (c MemoryCache) NumCounters() int64 {
	if c.MaxEntries <= 0 {
		return 0
	}
	entries := int64(c.MaxEntries)
	if entries > (1<<63-1)/10 {
		return 1<<63 - 1
	}
	return entries * 10
}
