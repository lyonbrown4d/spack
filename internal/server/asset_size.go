package server

import "github.com/lyonbrown4d/spack/internal/catalog"

func nonNegativeSize(size int64) (int64, bool) {
	if size < 0 {
		return 0, false
	}
	return size, true
}

func variantSize(variant *catalog.Variant) (int64, bool) {
	if variant == nil {
		return 0, false
	}
	return nonNegativeSize(variant.Size)
}

func assetSize(asset *catalog.Asset) (int64, bool) {
	if asset == nil {
		return 0, false
	}
	return nonNegativeSize(asset.Size)
}
