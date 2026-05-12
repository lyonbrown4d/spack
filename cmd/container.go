// Package cmd wires the CLI and application container.
package cmd

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/spack/internal/appmeta"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	spacklogger "github.com/lyonbrown4d/spack/internal/logger"
	"github.com/lyonbrown4d/spack/internal/metrics"
	"github.com/lyonbrown4d/spack/internal/runtime"
	"github.com/lyonbrown4d/spack/internal/task"
	"github.com/lyonbrown4d/spack/internal/validation"
	"github.com/samber/oops"
)

func createContainer(loadOptions config.LoadOptions, userModules ...dix.Module) (*dix.App, error) {
	allModules := cxlist.NewListWithCapacity[dix.Module](8 + len(userModules))
	allModules.Add(appmeta.Module,
		validation.Module,
		config.NewModule(loadOptions),
		spacklogger.Module,
		metrics.Module,
		catalog.Module,
		runtime.Module,
		task.Module,
	)
	allModules.Add(userModules...)
	instance := dix.New(
		"spack",
		dix.WithModules(allModules.Values()...),
		dix.WithRunStopTimeout(dix.DefaultRunStopTimeout),
	)
	err := instance.Validate()
	if err != nil {
		return nil, oops.In("command").Owner("container").Wrap(err)
	}
	return instance, nil
}
