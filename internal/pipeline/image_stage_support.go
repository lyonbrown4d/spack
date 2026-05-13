package pipeline

import (
	"cmp"
	"fmt"
	cxlist "github.com/arcgolabs/collectionx/list"
	cxset "github.com/arcgolabs/collectionx/set"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/media"
	"os"
)

func (s *imageStage) planFormats(asset *catalog.Asset, request Request) *cxlist.List[string] {
	var supported *cxlist.List[string]
	if s.engine != nil {
		supported = normalizeImageFormats(s.engine.SupportedTargetFormats())
	}

	formats := filterSupportedImageFormats(request.PreferredFormats, supported)
	if formats != nil && !formats.IsEmpty() {
		return formats
	}

	sourceFormat := media.ImageFormat(asset.MediaType)
	defaultFormats := filterSupportedImageFormats(s.cfg.ParsedFormats(), supported)
	if sourceFormat != "" {
		if defaultFormats == nil {
			defaultFormats = cxlist.NewList[string]()
		}
		defaultFormats.Add(sourceFormat)
	}
	return filterSupportedImageFormats(defaultFormats, supported)
}

func (s *imageStage) planWidths(asset *catalog.Asset, request Request, formats *cxlist.List[string]) *cxlist.List[int] {
	if request.PreferredWidths != nil && !request.PreferredWidths.IsEmpty() {
		return request.PreferredWidths
	}
	if request.PreferredFormats != nil && request.PreferredFormats.Len() > 0 {
		return cxlist.NewList[int](0)
	}

	widths := cxlist.NewList[int](s.cfg.ParsedWidths().Values()...)
	if shouldPlanOriginalFormatVariants(formats, media.ImageFormat(asset.MediaType)) {
		widths.Add(0)
	}
	if widths.IsEmpty() {
		return widths
	}

	unique := cxset.NewOrderedSet[int](widths.Sort(cmp.Compare[int]).Values()...)
	return cxlist.NewList[int](unique.Values()...)
}

func shouldPlanOriginalFormatVariants(formats *cxlist.List[string], sourceFormat string) bool {
	return formats.AnyMatch(func(_ int, format string) bool {
		return format != "" && format != sourceFormat
	})
}

func (s *imageStage) planTasks(asset *catalog.Asset, formats *cxlist.List[string], widths *cxlist.List[int]) *cxlist.List[Task] {
	if formats == nil || widths == nil {
		return nil
	}
	return cxlist.FlatMapList[string, Task](formats, func(_ int, format string) []Task {
		return cxlist.FilterMapList[int, Task](widths, func(_ int, width int) (Task, bool) {
			if !shouldCreateImageTask(asset, s.catalog, width, format) {
				return Task{}, false
			}
			return Task{
				AssetPath: asset.Path,
				Format:    format,
				Width:     width,
			}, true
		}).Values()
	})
}

func shouldCreateImageTask(asset *catalog.Asset, cat catalog.Catalog, width int, format string) bool {
	if width < 0 {
		return false
	}
	if width == 0 && format == media.ImageFormat(asset.MediaType) {
		return false
	}
	variant, ok := cat.FindImageVariant(asset.Path, format, width)
	if !ok {
		return true
	}
	return !hasImageVariant(variant, asset.SourceHash, width, format)
}

func resolveTargetFormat(task Task, asset *catalog.Asset) (string, error) {
	targetFormat := task.Format
	if targetFormat == "" {
		targetFormat = media.ImageFormat(asset.MediaType)
	}
	if task.Width < 0 {
		return "", ErrVariantSkipped
	}
	if task.Width == 0 && targetFormat == media.ImageFormat(asset.MediaType) {
		return "", ErrVariantSkipped
	}
	if targetFormat == "" {
		return "", fmt.Errorf("unsupported image format: %s", task.Format)
	}
	return targetFormat, nil
}

func shouldSkipImageArtifact(asset *catalog.Asset, result imageGenerateResult) bool {
	return result.Width == result.SourceWidth && result.MediaType == asset.MediaType && int64(len(result.Payload)) >= asset.Size
}

func hasImageVariant(variant *catalog.Variant, sourceHash string, width int, format string) bool {
	return isMatchingImageVariant(variant, sourceHash, width, format)
}

func isMatchingImageVariant(variant *catalog.Variant, sourceHash string, width int, format string) bool {
	if variant.Width != width {
		return false
	}
	if format != "" && variant.Format != format {
		return false
	}
	if sourceHash != "" && variant.SourceHash != "" && variant.SourceHash != sourceHash {
		return false
	}
	if variant.ArtifactPath == "" {
		return false
	}
	_, err := os.Stat(variant.ArtifactPath)
	return err == nil
}

func filterSupportedImageFormats(formats, supported *cxlist.List[string]) *cxlist.List[string] {
	normalized := normalizeImageFormats(formats)
	if normalized == nil || normalized.IsEmpty() || supported == nil || supported.IsEmpty() {
		return normalized
	}

	supportedSet := cxset.NewOrderedSet[string](supported.Values()...)
	return normalized.Where(func(_ int, format string) bool {
		return supportedSet.Contains(format)
	})
}
