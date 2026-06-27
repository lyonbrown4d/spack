package spackbundle

import "github.com/arcgolabs/dix"

// Module registers SPACK bundle services for DI consumers.
var Module = dix.NewModule("spackbundle",
	dix.WithModuleProviders(
		dix.Provider0(newBundleService),
		dix.Provider1(newBundleWriter),
		dix.Provider1(newBundleExtractor),
		dix.Provider1(newBundleReaderFactory),
	),
)
