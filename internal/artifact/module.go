// Package artifact manages generated artifact storage.
package artifact

import (
	"github.com/arcgolabs/dix"
	"github.com/daiyuang/spack/internal/config"
)

var Module = dix.NewModule("artifact",
	dix.WithModuleProviders(
		dix.ProviderErr1(func(cfg *config.Compression) (Store, error) {
			return newLocalStore(cfg.CacheDir)
		}),
	),
)
