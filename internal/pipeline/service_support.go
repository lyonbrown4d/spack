package pipeline

import (
	"cmp"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/samber/lo"
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

	normalized := cxlist.MapList[string, string](values, func(_ int, value string) string {
		return strings.ToLower(strings.TrimSpace(value))
	}).Where(func(_ int, value string) bool {
		return value != ""
	})
	normalized = cxlist.NewList[string](lo.Uniq[string](normalized.Values())...)
	if normalized.IsEmpty() {
		return nil
	}
	return normalized.Sort(strings.Compare)
}

func normalizeRequestInts(values *cxlist.List[int]) *cxlist.List[int] {
	if values == nil || values.IsEmpty() {
		return nil
	}

	normalized := values.Where(func(_ int, value int) bool {
		return value > 0
	})
	normalized = cxlist.NewList[int](lo.Uniq[int](normalized.Values())...)
	if normalized.IsEmpty() {
		return nil
	}
	return normalized.Sort(cmp.Compare[int])
}
