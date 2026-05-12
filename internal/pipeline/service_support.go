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

	normalized := cxset.NewOrderedSetWithCapacity[string](values.Len())
	values.Range(func(_ int, value string) bool {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			normalized.Add(value)
		}
		return true
	})
	if normalized.IsEmpty() {
		return nil
	}
	return cxlist.NewList[string](normalized.Values()...).Sort(strings.Compare)
}

func normalizeRequestInts(values *cxlist.List[int]) *cxlist.List[int] {
	if values == nil || values.IsEmpty() {
		return nil
	}

	normalized := cxset.NewOrderedSetWithCapacity[int](values.Len())
	values.Range(func(_ int, value int) bool {
		if value > 0 {
			normalized.Add(value)
		}
		return true
	})
	if normalized.IsEmpty() {
		return nil
	}
	return cxlist.NewList[int](normalized.Values()...).Sort(cmp.Compare[int])
}
