package resolver

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/catalog"
)

type imageVariantSelection struct {
	variants     *cxlist.List[*catalog.Variant]
	usable       variantUsabilityCache
	sourceHash   string
	format       string
	sourceFormat string
	width        int
}

func pickImageVariantForFormat(selection imageVariantSelection) *catalog.Variant {
	if selection.width <= 0 {
		return selection.findZeroWidthImageVariant()
	}
	return selection.findClosestWidthImageVariant()
}

func (s imageVariantSelection) findZeroWidthImageVariant() *catalog.Variant {
	var picked *catalog.Variant
	s.variants.Range(func(_ int, candidate *catalog.Variant) bool {
		if candidate.Width != 0 || !s.matches(candidate) {
			return true
		}
		picked = candidate
		return false
	})
	return picked
}

func (s imageVariantSelection) findClosestWidthImageVariant() *catalog.Variant {
	var smallestAbove *catalog.Variant
	var largestBelow *catalog.Variant
	s.variants.Range(func(_ int, candidate *catalog.Variant) bool {
		if !s.matches(candidate) {
			return true
		}
		smallestAbove, largestBelow = updateWidthMatches(candidate, s.width, smallestAbove, largestBelow)
		return true
	})
	if smallestAbove != nil {
		return smallestAbove
	}
	return largestBelow
}

func updateWidthMatches(
	candidate *catalog.Variant,
	width int,
	smallestAbove *catalog.Variant,
	largestBelow *catalog.Variant,
) (*catalog.Variant, *catalog.Variant) {
	if candidate.Width >= width {
		if smallestAbove == nil || candidate.Width < smallestAbove.Width {
			smallestAbove = candidate
		}
		return smallestAbove, largestBelow
	}
	if largestBelow == nil || candidate.Width > largestBelow.Width {
		largestBelow = candidate
	}
	return smallestAbove, largestBelow
}

func (s imageVariantSelection) matches(candidate *catalog.Variant) bool {
	if candidate == nil || (candidate.Width <= 0 && candidate.Format == "") {
		return false
	}
	if !s.usable.IsUsable(candidate, s.sourceHash) {
		return false
	}
	return s.format == "" || variantFormat(candidate, s.sourceFormat) == s.format
}
