package pipeline

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/pkg"
)

func normalizeRequestStrings(values *cxlist.List[string]) *cxlist.List[string] {
	return pkg.NilIfEmpty(
		pkg.NormalizeStringList(values, pkg.TrimLower, pkg.SortStrings),
	)
}

func normalizeRequestInts(values *cxlist.List[int]) *cxlist.List[int] {
	return pkg.NilIfEmpty(pkg.NormalizePositiveIntList(values))
}
