// Package mapx provides shared object mapping infrastructure.
package mapx

import (
	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/mapper"
)

var Module = dix.NewModule("mapx",
	dix.WithModuleProviders(
		dix.Provider0(New),
	),
)

func New() *mapper.Mapper {
	return mapper.New(
		mapper.WithFallbackTags("json", "yaml"),
	)
}
