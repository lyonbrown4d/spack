// Package spec defines the supported content-coding names and normalization helpers.
package spec

import (
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
)

type nameMask uint8

const (
	nameMaskBrotli nameMask = 1 << iota
	nameMaskZstd
	nameMaskGzip
)

func DefaultNames() *cxlist.List[string] {
	return cxlist.NewList[string]("br", "zstd", "gzip")
}

func IsSupported(name string) bool {
	_, ok := normalizedNameMask(name)
	return ok
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

	seen := nameMask(0)
	names := cxlist.NewListWithCapacity[string](values.Len())
	values.Range(func(_ int, raw string) bool {
		name, mask, ok := normalizedName(raw)
		if !ok || seen.has(mask) {
			return true
		}
		seen |= mask
		names.Add(name)
		return true
	})

	if names.IsEmpty() {
		return nil
	}
	return names
}

func normalizedName(raw string) (string, nameMask, bool) {
	name := strings.ToLower(strings.TrimSpace(raw))
	mask, ok := normalizedNameMask(name)
	return name, mask, ok
}

func normalizedNameMask(name string) (nameMask, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "br":
		return nameMaskBrotli, true
	case "zstd":
		return nameMaskZstd, true
	case "gzip":
		return nameMaskGzip, true
	default:
		return 0, false
	}
}

func (mask nameMask) has(value nameMask) bool {
	return mask&value != 0
}
