package source

import (
	"context"

	"github.com/arcgolabs/dix"
)

var Module = dix.NewModule("source",
	dix.WithModuleProviders(
		dix.ProviderErr2(NewLocalFS),
	),
	dix.WithModuleHooks(
		dix.OnStop(stopLocalFS),
	),
)

func stopLocalFS(_ context.Context, src *LocalFS) error {
	return src.Cleanup()
}
