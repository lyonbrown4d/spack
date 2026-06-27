package source

import (
	"context"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/spack/internal/config"
)

var Module = dix.NewModule("source",
	dix.WithModuleProviders(
		dix.Provider0(NewResolver),
		dix.Provider2(NewSourceFactory),
		dix.ProviderErr2(func(cfg *config.Assets, factory *SourceFactory) (*LocalFS, error) {
			return factory.LocalFS(cfg)
		}),
	),
	dix.WithModuleHooks(
		dix.OnStop(stopLocalFS),
	),
)

func stopLocalFS(_ context.Context, src *LocalFS) error {
	return src.Cleanup()
}
