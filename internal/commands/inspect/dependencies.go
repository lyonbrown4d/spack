package inspectcmd

import (
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
)

// ConfigResolver resolves config from CLI load options.
type ConfigResolver func(config.LoadOptions) (*config.Config, error)

// ScannerResolver resolves an asset scanner for an effective config.
type ScannerResolver func(*config.Config) (sourcecatalog.Scanner, error)

// Dependencies contains command dependencies supplied by a cmd package.
type Dependencies struct {
	ResolveConfig  ConfigResolver
	ResolveScanner ScannerResolver
}
