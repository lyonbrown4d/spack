// Package spec defines the supported content-coding names and normalization helpers.
package spec

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/pkg"
)

func DefaultNames() *cxlist.List[string] {
	return cxlist.NewList[string]("br", "zstd", "gzip")
}

func IsSupported(name string) bool {
	return NormalizeName(name) != ""
}

func ParseNames(raw string) *cxlist.List[string] {
	if pkg.IsBlank(raw) {
		return cxlist.NewList[string]()
	}
	return pkg.NilIfEmpty(
		pkg.NormalizeCSVStrings(raw, NormalizeName, pkg.PreserveOrder),
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
	return pkg.NilIfEmpty(
		pkg.NormalizeStringList(values, NormalizeName, pkg.PreserveOrder),
	)
}

func NormalizeName(raw string) string {
	switch pkg.TrimLower(raw) {
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
