package cmd

import (
	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/spack/internal/config"
)

// CreateContainerForTest exposes container construction for external tests.
func CreateContainerForTest(loadOptions config.LoadOptions, userModules ...dix.Module) (*dix.App, error) {
	return createContainer(loadOptions, userModules...)
}
