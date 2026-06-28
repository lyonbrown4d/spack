package sourcecatalog

import (
	"strings"

	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/pkg"
	"github.com/samber/mo"
)

func buildExplicitBundleVariants(
	filesByPath *cxmapping.Map[string, source.File],
	assets *cxmapping.Map[string, *catalog.Asset],
) *cxmapping.Map[string, *catalog.Variant] {
	variants := cxmapping.NewMapWithCapacity[string, *catalog.Variant](filesByPath.Len())
	sortedKeys[source.File](filesByPath).Range(func(_ int, filePath string) bool {
		file, _ := filesByPath.Get(filePath)
		if !isExplicitBundleVariantFile(file) {
			return true
		}
		asset, ok := explicitBundleVariantAsset(assets, file).Get()
		if !ok {
			return true
		}
		variant := buildExplicitBundleVariant(file, asset)
		variants.Set(variant.ID, variant)
		return true
	})
	return variants
}

func explicitBundleVariantAsset(
	assets *cxmapping.Map[string, *catalog.Asset],
	file source.File,
) mo.Option[*catalog.Asset] {
	assetPath := normalizeSourcePath(file.AssetPath)
	asset, ok := assets.GetOption(assetPath).Get()
	if !ok || asset == nil {
		return mo.None[*catalog.Asset]()
	}
	return mo.Some(asset)
}

func isExplicitBundleVariantFile(file source.File) bool {
	return normalizedExplicitBundleVariantKind(file.Kind) != ""
}

func buildExplicitBundleVariant(file source.File, asset *catalog.Asset) *catalog.Variant {
	kind := normalizedExplicitBundleVariantKind(file.Kind)
	variant := &catalog.Variant{
		ID:           catalog.VariantID(normalizeSourcePath(file.AssetPath), file.Encoding, file.Format, file.Width),
		AssetPath:    asset.Path,
		ArtifactPath: file.FullPath,
		Size:         file.Size,
		MediaType:    pkg.FirstNonBlank(file.MediaType, asset.MediaType),
		SourceHash:   pkg.FirstNonBlank(file.SourceHash, asset.SourceHash),
		ETag:         pkg.FirstNonBlank(file.ETag, asset.ETag),
		Encoding:     strings.TrimSpace(file.Encoding),
		Format:       strings.TrimSpace(file.Format),
		Width:        file.Width,
		Metadata:     explicitBundleVariantMetadata(file, asset.Path, kind),
	}
	return variant
}

func explicitBundleVariantMetadata(file source.File, assetPath, kind string) *cxmapping.Map[string, string] {
	return catalog.MetadataWithModTime(cxmapping.NewMapFrom(map[string]string{
		"stage":       kind,
		"source":      assetPath,
		"bundle_path": file.Path,
	}), file.ModTime)
}

func normalizedExplicitBundleVariantKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case BundleCompressionStage:
		return BundleCompressionStage
	case BundleImageStage:
		return BundleImageStage
	default:
		return ""
	}
}
