//go:build spack_libvips

package pipeline

import (
	"cmp"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxset "github.com/arcgolabs/collectionx/set"
	"github.com/samber/oops"
)

const maxImageInt64 = int64(1<<63 - 1)

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
	pixels, ok := imagePixelCount(source.width, source.height)
	if !ok {
		return oops.Wrapf(ErrVariantSkipped, "source image pixel count overflows")
	}
	if limits.MaxSourcePixels > 0 && pixels > limits.MaxSourcePixels {
		return oops.Wrapf(
			ErrVariantSkipped,
			"source image pixels %d exceed max source pixels %d",
			pixels,
			limits.MaxSourcePixels,
		)
	}
	decodeBytes, ok := checkedImageMul(pixels, 4)
	if !ok {
		return oops.Wrapf(ErrVariantSkipped, "source image decode bytes overflow")
	}
	if limits.MaxMemoryBytes > 0 && decodeBytes > limits.MaxMemoryBytes {
		return oops.Wrapf(
			ErrVariantSkipped,
			"source image decode bytes %d exceed max memory bytes %d",
			decodeBytes,
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
		height := scaledImageHeight(sourceHeight, width, sourceWidth)
		totalBytes = saturatingImageAdd(totalBytes, imageRawMemoryBytes(width, height))
	}
	return totalBytes
}

func imageRawMemoryBytes(width, height int) int64 {
	pixels, ok := imagePixelCount(width, height)
	if !ok {
		return maxImageInt64
	}
	bytes, ok := checkedImageMul(pixels, 4)
	if !ok {
		return maxImageInt64
	}
	return bytes
}

func imagePixelCount(width, height int) (int64, bool) {
	if width <= 0 || height <= 0 {
		return 0, true
	}
	return checkedImageMul(int64(width), int64(height))
}

func checkedImageMul(left, right int64) (int64, bool) {
	if left < 0 || right < 0 {
		return 0, false
	}
	if left == 0 || right == 0 {
		return 0, true
	}
	if left > maxImageInt64/right {
		return 0, false
	}
	return left * right, true
}

func saturatingImageAdd(left, right int64) int64 {
	if left > maxImageInt64-right {
		return maxImageInt64
	}
	return left + right
}

func scaledImageHeight(sourceHeight, width, sourceWidth int) int {
	if sourceHeight <= 0 || width <= 0 || sourceWidth <= 0 {
		return 1
	}
	numerator, ok := checkedImageMul(int64(sourceHeight), int64(width))
	if !ok {
		return sourceHeight
	}
	return max(1, int(numerator/int64(sourceWidth)))
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
