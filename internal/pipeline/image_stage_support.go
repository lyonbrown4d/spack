package pipeline

import (
	"cmp"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxset "github.com/arcgolabs/collectionx/set"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/media"
	"github.com/samber/oops"
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
	matched := false
	formats.Range(func(_ int, format string) bool {
		matched = format != "" && format != sourceFormat
		return !matched
	})
	return matched
}

func (s *imageStage) planTasks(asset *catalog.Asset, formats *cxlist.List[string], widths *cxlist.List[int]) *cxlist.List[Task] {
	variants := s.planImageVariants(asset, formats, widths)
	if variants.IsEmpty() {
		return cxlist.NewList[Task]()
	}
	variants = limitImageVariantTasks(variants, s.cfg.MaxOutputVariants)
	first, _ := variants.Get(0)
	return cxlist.NewList(Task{
		AssetPath:     asset.Path,
		Format:        first.Format,
		Width:         first.Width,
		ImageVariants: variants,
	})
}

func (s *imageStage) planImageVariants(
	asset *catalog.Asset,
	formats *cxlist.List[string],
	widths *cxlist.List[int],
) *cxlist.List[ImageVariantTask] {
	if formats == nil || widths == nil {
		return cxlist.NewList[ImageVariantTask]()
	}
	return cxlist.FlatMapList(formats, func(_ int, format string) []ImageVariantTask {
		return cxlist.FilterMapList(widths, func(_ int, width int) (ImageVariantTask, bool) {
			if !shouldCreateImageTask(asset, s.catalog, width, format) {
				return ImageVariantTask{}, false
			}
			return ImageVariantTask{
				Format: format,
				Width:  width,
			}, true
		}).Values()
	})
}

func limitImageVariantTasks(variants *cxlist.List[ImageVariantTask], limit int) *cxlist.List[ImageVariantTask] {
	if variants == nil || variants.IsEmpty() || limit <= 0 || variants.Len() <= limit {
		return variants
	}
	return cxlist.NewList(variants.Values()[:limit]...)
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
		return "", oops.Errorf("unsupported image format: %s", task.Format)
	}
	return targetFormat, nil
}

func (s *imageStage) shouldSkipImageArtifact(asset *catalog.Asset, result imageGenerateResult) bool {
	sourceBytes := asset.Size
	if result.SourceBytes > 0 {
		sourceBytes = result.SourceBytes
	}
	if sourceBytes <= 0 || len(result.Payload) == 0 {
		return false
	}
	return s.shouldSkipImageArtifactBySavings(asset, result, sourceBytes)
}

func (s *imageStage) shouldSkipImageArtifactBySavings(
	asset *catalog.Asset,
	result imageGenerateResult,
	sourceBytes int64,
) bool {
	outputBytes := int64(len(result.Payload))
	savedBytes := sourceBytes - outputBytes
	if savedBytes <= 0 {
		return sameSourceImageVariant(asset, result) || imageSavingPolicyEnabled(s.cfg)
	}
	if belowMinSavingBytes(s.cfg, savedBytes) {
		return true
	}
	if belowMinSavingRatio(s.cfg, savedBytes, sourceBytes) {
		return true
	}
	return false
}

func sameSourceImageVariant(asset *catalog.Asset, result imageGenerateResult) bool {
	return result.Width == result.SourceWidth && result.MediaType == asset.MediaType
}

func imageSavingPolicyEnabled(cfg *config.Image) bool {
	return cfg != nil && (cfg.MinSavingBytes > 0 || cfg.MinSavingRatio > 0)
}

func belowMinSavingBytes(cfg *config.Image, savedBytes int64) bool {
	return cfg != nil && cfg.MinSavingBytes > 0 && savedBytes < cfg.MinSavingBytes
}

func belowMinSavingRatio(cfg *config.Image, savedBytes, sourceBytes int64) bool {
	return cfg != nil && cfg.MinSavingRatio > 0 && float64(savedBytes)/float64(sourceBytes) < cfg.MinSavingRatio
}

func imageGenerateLimitsFromConfig(cfg *config.Image) imageGenerateLimits {
	if cfg == nil {
		return imageGenerateLimits{}
	}
	return imageGenerateLimits{
		MaxSourceBytes:  cfg.MaxSourceBytes,
		MaxSourcePixels: cfg.MaxSourcePixels,
		MaxWidth:        cfg.MaxWidth,
		MaxHeight:       cfg.MaxHeight,
		MaxMemoryBytes:  cfg.MaxMemoryBytes,
	}
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
	return cxlist.FilterList(normalized, func(_ int, format string) bool {
		return supportedSet.Contains(format)
	})
}
