package resolver

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/media"
)

func (r *Resolver) pickVariant(asset *catalog.Asset, encodings *cxlist.List[string]) (*catalog.Variant, error) {
	var picked *catalog.Variant
	var pickErr error
	encodings.Range(func(_ int, encoding string) bool {
		variant, ok, err := findEncodingVariantForRead(r.catalog, asset.Path, encoding)
		if err != nil {
			pickErr = err
			return false
		}
		if !ok || !isUsableVariant(variant, asset.SourceHash) {
			return true
		}
		picked = variant
		return false
	})
	return picked, pickErr
}

func (r *Resolver) pickImageVariant(asset *catalog.Asset, width int, formats *cxlist.List[string]) (*catalog.Variant, error) {
	sourceFormat := media.ImageFormat(asset.MediaType)
	if formats.IsEmpty() {
		formats = cxlist.NewList(sourceFormat)
	}

	usable := newVariantUsabilityCache()
	var picked *catalog.Variant
	var pickErr error
	formats.Range(func(_ int, format string) bool {
		variant, err := r.pickImageVariantForFormat(asset, imageVariantSelection{
			usable:       usable,
			sourceHash:   asset.SourceHash,
			format:       format,
			sourceFormat: sourceFormat,
			width:        width,
		})
		if err != nil {
			pickErr = err
			return false
		}
		picked = variant
		return picked == nil
	})
	return picked, pickErr
}

func (r *Resolver) pickImageVariantForFormat(
	asset *catalog.Asset,
	selection imageVariantSelection,
) (*catalog.Variant, error) {
	if selection.width <= 0 {
		variant, ok, err := findImageVariantForRead(r.catalog, asset.Path, selection.format, 0)
		if err != nil || !ok || !selection.matches(variant) {
			return nil, err
		}
		return variant, nil
	}

	variants, err := listImageVariantsForRead(r.catalog, asset.Path, selection.format)
	if err != nil || variants.IsEmpty() {
		return nil, err
	}
	selection.variants = variants
	return pickImageVariantForFormat(selection), nil
}
