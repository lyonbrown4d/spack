// Package spec defines the supported content-coding names and normalization helpers.
package spec

import (
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxset "github.com/arcgolabs/collectionx/set"
)

func DefaultNames() *cxlist.List[string] {
	return cxlist.NewList[string]("br", "zstd", "gzip")
}

func IsSupported(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "br", "zstd", "gzip":
		return true
	default:
		return false
	}
}

func ParseNames(raw string) *cxlist.List[string] {
	if strings.TrimSpace(raw) == "" {
		return cxlist.NewList[string]()
	}
	return NormalizeNames(cxlist.NewList[string](strings.Split(raw, ",")...))
}

func ResolveNames(raw string) *cxlist.List[string] {
	names := ParseNames(raw)
	if names.IsEmpty() {
		return DefaultNames()
	}
	return names
}

func NormalizeNames(values *cxlist.List[string]) *cxlist.List[string] {
	if values == nil || values.IsEmpty() {
		return nil
	}

	names := cxlist.FilterMapList[string, string](values, func(_ int, raw string) (string, bool) {
		name := strings.ToLower(strings.TrimSpace(raw))
		return name, IsSupported(name)
	})

	normalized := cxset.NewOrderedSet[string](names.Values()...)
	if normalized.IsEmpty() {
		return nil
	}
	return cxlist.NewList[string](normalized.Values()...)
}
