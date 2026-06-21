// Package spec defines the supported content-coding names and normalization helpers.
package spec

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/normalizex"
)

func DefaultNames() *cxlist.List[string] {
	return cxlist.NewList[string]("br", "zstd", "gzip")
}

func IsSupported(name string) bool {
	return NormalizeName(name) != ""
}

func ParseNames(raw string) *cxlist.List[string] {
	if normalizex.IsBlank(raw) {
		return cxlist.NewList[string]()
	}
	return normalizex.NilIfEmpty(
		normalizex.NormalizeCSVStrings(raw, NormalizeName, normalizex.PreserveOrder),
	)
}

func ResolveNames(raw string) *cxlist.List[string] {
	names := ParseNames(raw)
	if names == nil || names.IsEmpty() {
		return DefaultNames()
	}
	return names
}

func NormalizeNames(values *cxlist.List[string]) *cxlist.List[string] {
	return normalizex.NilIfEmpty(
		normalizex.NormalizeStringList(values, NormalizeName, normalizex.PreserveOrder),
	)
}

func NormalizeName(raw string) string {
	switch normalizex.TrimLower(raw) {
	case "br":
		return "br"
	case "zstd":
		return "zstd"
	case "gzip":
		return "gzip"
	default:
		return ""
	}
}
