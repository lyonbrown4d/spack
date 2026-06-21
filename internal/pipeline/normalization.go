package pipeline

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/normalizex"
)

func normalizeRequestStrings(values *cxlist.List[string]) *cxlist.List[string] {
	return normalizex.NilIfEmpty(
		normalizex.NormalizeStringList(values, normalizex.TrimLower, normalizex.SortStrings),
	)
}

func normalizeRequestInts(values *cxlist.List[int]) *cxlist.List[int] {
	return normalizex.NilIfEmpty(normalizex.NormalizePositiveIntList(values))
}
