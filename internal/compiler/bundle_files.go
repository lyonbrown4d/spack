package compiler

import (
	"cmp"
	"fmt"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/lo"
	"github.com/samber/mo"
	"github.com/samber/oops"
)

func upsertCompileSnapshot(cat catalog.Catalog, snapshot sourcecatalog.Snapshot) error {
	var upsertErr error
	snapshot.Assets.Range(func(_ string, asset *catalog.Asset) bool {
		if err := cat.UpsertAsset(asset); err != nil {
			upsertErr = oops.Wrapf(err, "upsert compile asset %q", asset.Path)
			return false
		}
		return true
	})
	if upsertErr != nil {
		return upsertErr
	}
	snapshot.Variants.Range(func(_ string, variant *catalog.Variant) bool {
		if err := cat.UpsertVariant(variant); err != nil {
			upsertErr = oops.Wrapf(err, "upsert compile variant %q", variant.ID)
			return false
		}
		return true
	})
	return upsertErr
}

func bundleFilesFromCatalog(root, output string, cat catalog.Catalog) []spackbundle.File {
	assets := cat.AllAssets()
	variants := cat.AllVariants()
	excludedOutput := normalizedOptionalPath(output)
	assetFiles := lo.FilterMap(assets.Values(), func(asset *catalog.Asset, _ int) (spackbundle.File, bool) {
		return bundleAssetFile(asset, excludedOutput)
	})
	variantFiles := lo.FilterMap(variants.Values(), func(variant *catalog.Variant, _ int) (spackbundle.File, bool) {
		return bundleVariantFile(root, variant, excludedOutput)
	})
	files := lo.Concat(assetFiles, variantFiles)
	slices.SortFunc(files, func(left, right spackbundle.File) int {
		return cmp.Compare(left.Path, right.Path)
	})
	return files
}

func bundleAssetFile(asset *catalog.Asset, excludedOutput mo.Option[string]) (spackbundle.File, bool) {
	if asset == nil || shouldExcludeBundlePath(asset.FullPath, excludedOutput) {
		return spackbundle.File{}, false
	}
	return spackbundle.File{
		Path:       asset.Path,
		FullPath:   asset.FullPath,
		Kind:       "asset",
		Size:       asset.Size,
		MediaType:  asset.MediaType,
		SourceHash: asset.SourceHash,
		ETag:       asset.ETag,
	}, true
}

func bundleVariantFile(
	root string,
	variant *catalog.Variant,
	excludedOutput mo.Option[string],
) (spackbundle.File, bool) {
	if variant == nil || strings.TrimSpace(variant.ArtifactPath) == "" ||
		shouldExcludeBundlePath(variant.ArtifactPath, excludedOutput) {
		return spackbundle.File{}, false
	}
	kind := bundleVariantKind(variant)
	if kind == "" {
		return spackbundle.File{}, false
	}
	bundlePath := bundleVariantPath(root, variant)
	allowExternal := false
	if kind != sourcecatalog.SourceSidecarStage {
		bundlePath = bundleGeneratedVariantPath(kind, variant)
		allowExternal = true
	}
	if bundlePath == "" {
		return spackbundle.File{}, false
	}
	return spackbundle.File{
		Path:          bundlePath,
		FullPath:      variant.ArtifactPath,
		Kind:          kind,
		Size:          variant.Size,
		MediaType:     variant.MediaType,
		SourceHash:    variant.SourceHash,
		ETag:          variant.ETag,
		AssetPath:     variant.AssetPath,
		Encoding:      variant.Encoding,
		Format:        variant.Format,
		Width:         variant.Width,
		AllowExternal: allowExternal,
	}, true
}

func bundleVariantKind(variant *catalog.Variant) string {
	if sourcecatalog.IsSourceSidecarVariant(variant) {
		return sourcecatalog.SourceSidecarStage
	}
	if variant == nil {
		return ""
	}
	if variant.Metadata != nil {
		switch strings.TrimSpace(variant.Metadata.GetOrDefault("stage", "")) {
		case sourcecatalog.BundleCompressionStage:
			return sourcecatalog.BundleCompressionStage
		case sourcecatalog.BundleImageStage:
			return sourcecatalog.BundleImageStage
		}
	}
	if strings.TrimSpace(variant.Encoding) != "" {
		return sourcecatalog.BundleCompressionStage
	}
	if strings.TrimSpace(variant.Format) != "" || variant.Width > 0 {
		return sourcecatalog.BundleImageStage
	}
	return ""
}

func bundleGeneratedVariantPath(kind string, variant *catalog.Variant) string {
	assetPath := strings.TrimPrefix(path.Clean(filepath.ToSlash(strings.TrimSpace(variant.AssetPath))), "/")
	if assetPath == "." || assetPath == "" {
		assetPath = sanitizeBundlePathComponent(variant.ID)
	}
	if assetPath == "" {
		return ""
	}
	base := path.Join("generated", sanitizeBundlePathComponent(kind), assetPath)
	encoding := strings.TrimSpace(variant.Encoding)
	format := strings.TrimSpace(variant.Format)
	parts := lo.Compact([]string{
		lo.Ternary(encoding != "", "encoding-"+sanitizeBundlePathComponent(encoding), ""),
		lo.Ternary(format != "", "format-"+sanitizeBundlePathComponent(format), ""),
		lo.Ternary(variant.Width > 0, fmt.Sprintf("w%d", variant.Width), ""),
	})
	if len(parts) > 0 {
		base += "." + strings.Join(parts, ".")
	}
	if ext := filepath.Ext(variant.ArtifactPath); ext != "" {
		base += ext
	}
	return base
}

func sanitizeBundlePathComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"\\", "_",
		"/", "_",
		":", "_",
		"|", "_",
		"=", "-",
		" ", "_",
	)
	return replacer.Replace(value)
}

func shouldExcludeBundlePath(rawPath string, excludedOutput mo.Option[string]) bool {
	output, ok := excludedOutput.Get()
	return ok && sameFilesystemPath(rawPath, output)
}

func normalizedOptionalPath(rawPath string) mo.Option[string] {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return mo.None[string]()
	}
	absolute, err := filepath.Abs(filepath.Clean(rawPath))
	if err != nil {
		return mo.None[string]()
	}
	return mo.Some(absolute)
}

func sameFilesystemPath(left, right string) bool {
	left, leftOK := normalizedOptionalPath(left).Get()
	right, rightOK := normalizedOptionalPath(right).Get()
	if !leftOK || !rightOK {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func bundleVariantPath(root string, variant *catalog.Variant) string {
	if variant == nil {
		return ""
	}
	if rel, err := filepath.Rel(root, variant.ArtifactPath); err == nil {
		return filepath.ToSlash(rel)
	}
	return variant.ID
}
