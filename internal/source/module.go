package source

import "github.com/arcgolabs/dix"

var Module = dix.NewModule("source",
	dix.WithModuleProviders(
		dix.ProviderErr2(NewLocalFS),
	),
)
