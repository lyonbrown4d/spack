// Package sourcecatalog scans source files into catalog assets and source-managed variants.
package sourcecatalog

import (
	"github.com/arcgolabs/dix"
)

var Module = dix.NewModule("sourcecatalog",
	dix.WithModuleProviders(
		dix.Provider3(NewScannerWithAssets),
	),
)
