package config

import (
	"time"

	"github.com/lyonbrown4d/spack/internal/validation"
)

type Frontend struct {
	ResourceHints  ResourceHints  `koanf:"resource_hints"  validate:"required"`
	ImmutableCache ImmutableCache `koanf:"immutable_cache" validate:"required"`
}

type ResourceHints struct {
	Enable         bool `koanf:"enable"`
	EarlyHints     bool `koanf:"early_hints"`
	MaxLinks       int  `koanf:"max_links"        validate:"gte=0"`
	MaxHeaderBytes int  `koanf:"max_header_bytes" validate:"gte=0"`
}

type ImmutableCache struct {
	Enable bool   `koanf:"enable"`
	MaxAge string `koanf:"max_age" validate:"omitempty,spack_flexible_duration"`
}

func (h ResourceHints) Enabled() bool {
	return h.Enable && h.LinkLimit() > 0 && h.HeaderByteLimit() > 0
}

func (h ResourceHints) LinkLimit() int {
	if h.MaxLinks > 0 {
		return h.MaxLinks
	}
	return 0
}

func (h ResourceHints) HeaderByteLimit() int {
	if h.MaxHeaderBytes > 0 {
		return h.MaxHeaderBytes
	}
	return 0
}

func (c ImmutableCache) ParsedMaxAge() time.Duration {
	return validation.ParseFlexibleDuration(c.MaxAge)
}
