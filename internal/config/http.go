package config

import (
	"strconv"
	"strings"
	"time"
)

type HTTP struct {
	Port               int         `koanf:"port"                 validate:"gte=1,lte=65535"`
	LowMemory          bool        `koanf:"low_memory"`
	Prefork            bool        `koanf:"prefork"`
	ExposeServerHeader bool        `koanf:"expose_server_header"`
	MemoryCache        MemoryCache `koanf:"memory_cache"         validate:"required"`
	RequestLogDetail   bool        `koanf:"request_log_detail"`
}

type MemoryCache struct {
	Enable      bool   `koanf:"enable"`
	Warmup      bool   `koanf:"warmup"`
	MaxEntries  int    `koanf:"max_entries"   validate:"gte=0"`
	MaxBytes    int64  `koanf:"max_bytes"     validate:"gte=0"`
	MaxFileSize int64  `koanf:"max_file_size" validate:"gte=0"`
	TTL         string `koanf:"ttl"           validate:"omitempty,spack_duration"`
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
