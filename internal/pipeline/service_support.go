package pipeline

import (
	"cmp"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxset "github.com/arcgolabs/collectionx/set"
	"strings"
)

func requestKey(request Request) string {
	assetPath := strings.TrimSpace(request.AssetPath)
	encodings := normalizeRequestStrings(request.PreferredEncodings)
	formats := normalizeRequestStrings(request.PreferredFormats)
	widths := normalizeRequestInts(request.PreferredWidths)
	return buildRequestKey(assetPath, encodings, formats, widths)
}

func normalizeRequestStrings(values *cxlist.List[string]) *cxlist.List[string] {
	if values == nil || values.IsEmpty() {
		return nil
	}

	items := cxlist.FilterMapList[string, string](values, func(_ int, value string) (string, bool) {
		value = strings.ToLower(strings.TrimSpace(value))
		return value, value != ""
	})

	normalized := cxset.NewOrderedSet[string](items.Values()...)
	if normalized.IsEmpty() {
		return nil
	}
	return cxlist.NewList[string](normalized.Values()...).Sort(strings.Compare)
}

func normalizeRequestInts(values *cxlist.List[int]) *cxlist.List[int] {
	if values == nil || values.IsEmpty() {
		return nil
	}

	positive := values.Where(func(_ int, value int) bool {
		return value > 0
	})

	normalized := cxset.NewOrderedSet[int](positive.Values()...)
	if normalized.IsEmpty() {
		return nil
	}
	return cxlist.NewList[int](normalized.Values()...).Sort(cmp.Compare[int])
}
