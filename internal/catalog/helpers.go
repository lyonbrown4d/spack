package catalog

import (
	"strconv"
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
)

func cloneAsset(asset *Asset) *Asset {
	if asset == nil {
		return nil
	}

	cloned := *asset
	cloned.Metadata = CloneMetadata(asset.Metadata)
	return &cloned
}

func cloneVariant(variant *Variant) *Variant {
	if variant == nil {
		return nil
	}

	cloned := *variant
	cloned.Metadata = CloneMetadata(variant.Metadata)
	return &cloned
}

func cloneVariants(variants *cxlist.List[*Variant]) *cxlist.List[*Variant] {
	return cxlist.MapList[*Variant, *Variant](variants, func(_ int, variant *Variant) *Variant {
		return cloneVariant(variant)
	})
}

func prepareAsset(asset *Asset) *Asset {
	cloned := cloneAsset(asset)
	if cloned == nil {
		return nil
	}
	cloned.Metadata = EnsureMetadataModTime(cloned.Metadata, cloned.FullPath)
	return cloned
}

func prepareVariant(variant *Variant) *Variant {
	cloned := cloneVariant(variant)
	if cloned == nil {
		return nil
	}
	cloned.Metadata = EnsureMetadataModTime(cloned.Metadata, cloned.ArtifactPath)
	return cloned
}

func VariantID(assetPath, encoding, format string, width int) string {
	id := strings.TrimSpace(assetPath)
	encoding = strings.TrimSpace(encoding)
	format = strings.TrimSpace(format)
	if encoding != "" {
		id += "|encoding=" + encoding
	}
	if format != "" {
		id += "|format=" + format
	}
	if width > 0 {
		id += "|width=" + strconv.Itoa(width)
	}
	return id
}

func defaultVariantID(variant *Variant) string {
	return VariantID(variant.AssetPath, variant.Encoding, variant.Format, variant.Width)
}

func cloneVariantRecords(records *cxlist.List[*variantRecord]) *cxlist.List[*Variant] {
	return cxlist.MapList[*variantRecord, *Variant](records, func(_ int, record *variantRecord) *Variant {
		return cloneVariant(record.Variant)
	})
}
