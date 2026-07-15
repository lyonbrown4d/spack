//go:build spack_libvips

package pipeline

import (
	"cmp"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxset "github.com/arcgolabs/collectionx/set"
	"github.com/samber/oops"
)

type imageSourceSnapshot struct {
	width  int
	height int
	bytes  int64
}

func imageSourceBytes(size int64, limits imageGenerateLimits) (int64, error) {
	if size <= 0 {
		return 0, oops.Wrapf(ErrVariantSkipped, "source image bytes %d are invalid", size)
	}
	if limits.MaxSourceBytes > 0 && size > limits.MaxSourceBytes {
		return 0, oops.Wrapf(
			ErrVariantSkipped,
			"source image bytes %d exceed max source bytes %d",
			size,
			limits.MaxSourceBytes,
		)
	}
	return size, nil
}

func validateImageSourceLimits(source imageSourceSnapshot, limits imageGenerateLimits) error {
	if source.width <= 0 || source.height <= 0 {
		return oops.Wrapf(
			ErrVariantSkipped,
			"source image dimensions %dx%d are invalid",
			source.width,
			source.height,
		)
	}
	if limits.MaxWidth > 0 && source.width > limits.MaxWidth {
		return oops.Wrapf(
			ErrVariantSkipped,
			"source image width %d exceeds max width %d",
			source.width,
			limits.MaxWidth,
		)
	}
	if limits.MaxHeight > 0 && source.height > limits.MaxHeight {
		return oops.Wrapf(
			ErrVariantSkipped,
			"source image height %d exceeds max height %d",
			source.height,
			limits.MaxHeight,
		)
	}
	return validateImagePixelLimits(source, limits)
}

func validateImagePixelLimits(source imageSourceSnapshot, limits imageGenerateLimits) error {
	pixels := int64(source.width) * int64(source.height)
	if limits.MaxSourcePixels > 0 && pixels > limits.MaxSourcePixels {
		return oops.Wrapf(
			ErrVariantSkipped,
			"source image pixels %d exceed max source pixels %d",
			pixels,
			limits.MaxSourcePixels,
		)
	}
	if limits.MaxMemoryBytes > 0 && pixels*4 > limits.MaxMemoryBytes {
		return oops.Wrapf(
			ErrVariantSkipped,
			"source image decode bytes %d exceed max memory bytes %d",
			pixels*4,
			limits.MaxMemoryBytes,
		)
	}
	return nil
}

func estimateImageBatchMemoryBytes(
	sourceWidth int,
	sourceHeight int,
	variants *cxlist.List[imageVariantGenerateRequest],
) int64 {
	totalBytes := imageRawMemoryBytes(sourceWidth, sourceHeight)
	if variants == nil || variants.IsEmpty() {
		return totalBytes
	}
	widths := uniqueImageOutputWidths(sourceWidth, variants)
	for _, width := range widths {
		if width == sourceWidth {
			continue
		}
		height := max(1, sourceHeight*width/sourceWidth)
		totalBytes += imageRawMemoryBytes(width, height)
	}
	return totalBytes
}

func imageRawMemoryBytes(width, height int) int64 {
	if width <= 0 || height <= 0 {
		return 0
	}
	return int64(width) * int64(height) * 4
}

func uniqueImageOutputWidths(sourceWidth int, variants *cxlist.List[imageVariantGenerateRequest]) []int {
	widths := cxset.NewSetWithCapacity[int](variants.Len() + 1)
	variants.Range(func(_ int, variant imageVariantGenerateRequest) bool {
		widths.Add(normalizedImageOutputWidth(sourceWidth, variant.TargetWidth))
		return true
	})
	return cxlist.NewList(widths.Values()...).Sort(func(left, right int) int {
		return cmp.Compare(right, left)
	}).Values()
}

func normalizedImageOutputWidth(sourceWidth, targetWidth int) int {
	if targetWidth <= 0 || targetWidth >= sourceWidth {
		return sourceWidth
	}
	return targetWidth
}

func clampImageQuality(quality int) int {
	if quality < 1 {
		return 1
	}
	if quality > 100 {
		return 100
	}
	return quality
}
