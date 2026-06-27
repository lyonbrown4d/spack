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
	files := make([]spackbundle.File, 0, assets.Len()+variants.Len())
	excludedOutput, hasExcludedOutput := normalizedOptionalPath(output)
	assets.Range(func(_ int, asset *catalog.Asset) bool {
		files = appendBundleAssetFile(files, asset, excludedOutput, hasExcludedOutput)
		return true
	})
	variants.Range(func(_ int, variant *catalog.Variant) bool {
		files = appendBundleVariantFile(files, root, variant, excludedOutput, hasExcludedOutput)
		return true
	})
	slices.SortFunc(files, func(left, right spackbundle.File) int {
		return cmp.Compare(left.Path, right.Path)
	})
	return files
}

func appendBundleAssetFile(
	files []spackbundle.File,
	asset *catalog.Asset,
	excludedOutput string,
	hasExcludedOutput bool,
) []spackbundle.File {
	if asset == nil || shouldExcludeBundlePath(asset.FullPath, excludedOutput, hasExcludedOutput) {
		return files
	}
	return append(files, spackbundle.File{
		Path:       asset.Path,
		FullPath:   asset.FullPath,
		Kind:       "asset",
		Size:       asset.Size,
		MediaType:  asset.MediaType,
		SourceHash: asset.SourceHash,
		ETag:       asset.ETag,
	})
}

func appendBundleVariantFile(
	files []spackbundle.File,
	root string,
	variant *catalog.Variant,
	excludedOutput string,
	hasExcludedOutput bool,
) []spackbundle.File {
	if variant == nil || strings.TrimSpace(variant.ArtifactPath) == "" ||
		shouldExcludeBundlePath(variant.ArtifactPath, excludedOutput, hasExcludedOutput) {
		return files
	}
	kind := bundleVariantKind(variant)
	if kind == "" {
		return files
	}
	bundlePath := bundleVariantPath(root, variant)
	allowExternal := false
	if kind != sourcecatalog.SourceSidecarStage {
		bundlePath = bundleGeneratedVariantPath(kind, variant)
		allowExternal = true
	}
	if bundlePath == "" {
		return files
	}
	return append(files, spackbundle.File{
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
	})
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
	parts := make([]string, 0, 3)
	if encoding := strings.TrimSpace(variant.Encoding); encoding != "" {
		parts = append(parts, "encoding-"+sanitizeBundlePathComponent(encoding))
	}
	if format := strings.TrimSpace(variant.Format); format != "" {
		parts = append(parts, "format-"+sanitizeBundlePathComponent(format))
	}
	if variant.Width > 0 {
		parts = append(parts, fmt.Sprintf("w%d", variant.Width))
	}
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

func shouldExcludeBundlePath(rawPath, excludedOutput string, hasExcludedOutput bool) bool {
	return hasExcludedOutput && sameFilesystemPath(rawPath, excludedOutput)
}

func normalizedOptionalPath(rawPath string) (string, bool) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", false
	}
	absolute, err := filepath.Abs(filepath.Clean(rawPath))
	if err != nil {
		return "", false
	}
	return absolute, true
}

func sameFilesystemPath(left, right string) bool {
	left, leftOK := normalizedOptionalPath(left)
	right, rightOK := normalizedOptionalPath(right)
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
