package resolver

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/media"
	"github.com/samber/lo"
	"github.com/samber/oops"
	"strings"
)

var supportedImageFormats = media.SupportedImageFormats()

type encodingVariantViewCatalog interface {
	FindEncodingVariantView(assetPath, encoding string) (*catalog.Variant, bool)
}

type encodingVariantViewResultCatalog interface {
	FindEncodingVariantViewResult(assetPath, encoding string) (*catalog.Variant, bool, error)
}

type imageVariantViewCatalog interface {
	FindImageVariantView(assetPath, format string, width int) (*catalog.Variant, bool)
	ListImageVariantsView(assetPath, format string) *cxlist.List[*catalog.Variant]
}

type imageVariantViewResultCatalog interface {
	FindImageVariantViewResult(assetPath, format string, width int) (*catalog.Variant, bool, error)
	ListImageVariantsViewResult(assetPath, format string) (*cxlist.List[*catalog.Variant], error)
}

type assetViewCatalog interface {
	FindAssetView(assetPath string) (*catalog.Asset, bool)
}

type assetViewResultCatalog interface {
	FindAssetViewResult(assetPath string) (*catalog.Asset, bool, error)
}

func findAssetForRead(cat catalog.Catalog, assetPath string) (*catalog.Asset, bool, error) {
	if checkedCat, ok := cat.(assetViewResultCatalog); ok {
		asset, ok, err := checkedCat.FindAssetViewResult(assetPath)
		return asset, ok, wrapCatalogReadErr(err)
	}
	if fastCat, ok := cat.(assetViewCatalog); ok {
		asset, ok := fastCat.FindAssetView(assetPath)
		return asset, ok, nil
	}
	asset, ok := cat.FindAsset(assetPath)
	return asset, ok, nil
}

func findEncodingVariantForRead(cat catalog.Catalog, assetPath, encoding string) (*catalog.Variant, bool, error) {
	if checkedCat, ok := cat.(encodingVariantViewResultCatalog); ok {
		variant, ok, err := checkedCat.FindEncodingVariantViewResult(assetPath, encoding)
		return variant, ok, wrapCatalogReadErr(err)
	}
	if fastCat, ok := cat.(encodingVariantViewCatalog); ok {
		variant, ok := fastCat.FindEncodingVariantView(assetPath, encoding)
		return variant, ok, nil
	}
	variant, ok := cat.FindEncodingVariant(assetPath, encoding)
	return variant, ok, nil
}

func findImageVariantForRead(cat catalog.Catalog, assetPath, format string, width int) (*catalog.Variant, bool, error) {
	if checkedCat, ok := cat.(imageVariantViewResultCatalog); ok {
		variant, ok, err := checkedCat.FindImageVariantViewResult(assetPath, format, width)
		return variant, ok, wrapCatalogReadErr(err)
	}
	if fastCat, ok := cat.(imageVariantViewCatalog); ok {
		variant, ok := fastCat.FindImageVariantView(assetPath, format, width)
		return variant, ok, nil
	}
	variant, ok := cat.FindImageVariant(assetPath, format, width)
	return variant, ok, nil
}

func listImageVariantsForRead(cat catalog.Catalog, assetPath, format string) (*cxlist.List[*catalog.Variant], error) {
	if checkedCat, ok := cat.(imageVariantViewResultCatalog); ok {
		variants, err := checkedCat.ListImageVariantsViewResult(assetPath, format)
		return variants, wrapCatalogReadErr(err)
	}
	if fastCat, ok := cat.(imageVariantViewCatalog); ok {
		return fastCat.ListImageVariantsView(assetPath, format), nil
	}
	return cat.ListImageVariants(assetPath, format), nil
}

func wrapCatalogReadErr(err error) error {
	if err == nil {
		return nil
	}
	return oops.In("resolver").Owner("catalog").Wrap(err)
}

type variantUsabilityCache struct {
	values *cxmapping.Map[string, bool]
}

func newVariantUsabilityCache() variantUsabilityCache {
	return variantUsabilityCache{values: cxmapping.NewMap[string, bool]()}
}

func (cache variantUsabilityCache) IsUsable(variant *catalog.Variant, assetSourceHash string) bool {
	if variant == nil {
		return false
	}

	key := variant.ID
	if key == "" {
		key = variant.ArtifactPath
	}

	if cache.values == nil {
		return isUsableVariant(variant, assetSourceHash)
	}

	if usable, ok := cache.values.Get(key); ok {
		return usable
	}

	usable := isUsableVariant(variant, assetSourceHash)
	cache.values.Set(key, usable)
	return usable
}

func isUsableVariant(variant *catalog.Variant, assetSourceHash string) bool {
	if variant == nil || strings.TrimSpace(variant.ArtifactPath) == "" {
		return false
	}
	if assetSourceHash != "" && variant.SourceHash != "" && variant.SourceHash != assetSourceHash {
		return false
	}
	return true
}

func variantFormat(variant *catalog.Variant, sourceFormat string) string {
	if variant == nil {
		return sourceFormat
	}
	return firstNonEmpty(variant.Format, sourceFormat)
}

func firstNonEmpty(values ...string) string {
	return lo.FindOrElse[string](values, "", func(value string) bool {
		return strings.TrimSpace(value) != ""
	})
}

func preferredWidths(width int) *cxlist.List[int] {
	return PreferredWidths(width)
}

func PreferredWidths(width int) *cxlist.List[int] {
	if width <= 0 {
		return nil
	}
	return cxlist.NewList[int](width)
}

func preferredImageFormats(
	acceptHeader,
	explicitFormat,
	sourceMediaType string,
) *cxlist.List[string] {
	return PreferredImageFormats(acceptHeader, explicitFormat, sourceMediaType)
}

func PreferredImageFormats(
	acceptHeader,
	explicitFormat,
	sourceMediaType string,
) *cxlist.List[string] {
	if explicitFormat != "" {
		return cxlist.NewList[string](explicitFormat)
	}
	if !media.IsImageMediaType(sourceMediaType) {
		return nil
	}
	return parseAcceptImageFormats(acceptHeader, media.ImageFormat(sourceMediaType), supportedImageFormats)
}
