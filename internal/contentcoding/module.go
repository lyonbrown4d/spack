// Package contentcoding provides configurable content-coding strategy registration.
package contentcoding

import (
	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/spack/internal/config"
)

var Module = dix.NewModule("contentcoding",
	dix.WithModuleProviders(
		dix.Provider1(func(cfg *config.Compression) Registry {
			return NewRegistry(Options{
				BrotliQuality: cfg.BrotliQuality,
				GzipLevel:     cfg.GzipLevel,
				ZstdLevel:     cfg.ZstdLevel,
			}, cfg.NormalizedEncodings())
		}),
	),
)
