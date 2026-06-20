//go:build spack_libvips

package pipeline

import (
	"fmt"
	"os"
	"sort"

	cxlist "github.com/arcgolabs/collectionx/list"
)

type imageSourceSnapshot struct {
	width  int
	height int
	bytes  int64
}

func imageSourceBytes(path string, limits imageGenerateLimits) (int64, error) {
	// #nosec G304 -- path comes from scanned assets rooted under configured sources.
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat source image: %w", err)
	}
	size := info.Size()
	if limits.MaxSourceBytes > 0 && size > limits.MaxSourceBytes {
		return 0, fmt.Errorf(
			"source image bytes %d exceed max source bytes %d: %w",
			size,
			limits.MaxSourceBytes,
			ErrVariantSkipped,
		)
	}
	return size, nil
}

func validateImageSourceLimits(source imageSourceSnapshot, limits imageGenerateLimits) error {
	if source.width <= 0 || source.height <= 0 {
		return fmt.Errorf(
			"source image dimensions %dx%d are invalid: %w",
			source.width,
			source.height,
			ErrVariantSkipped,
		)
	}
	if limits.MaxWidth > 0 && source.width > limits.MaxWidth {
		return fmt.Errorf(
			"source image width %d exceeds max width %d: %w",
			source.width,
			limits.MaxWidth,
			ErrVariantSkipped,
		)
	}
	if limits.MaxHeight > 0 && source.height > limits.MaxHeight {
		return fmt.Errorf(
			"source image height %d exceeds max height %d: %w",
			source.height,
			limits.MaxHeight,
			ErrVariantSkipped,
		)
	}
	return validateImagePixelLimits(source, limits)
}

func validateImagePixelLimits(source imageSourceSnapshot, limits imageGenerateLimits) error {
	pixels := int64(source.width) * int64(source.height)
	if limits.MaxSourcePixels > 0 && pixels > limits.MaxSourcePixels {
		return fmt.Errorf(
			"source image pixels %d exceed max source pixels %d: %w",
			pixels,
			limits.MaxSourcePixels,
			ErrVariantSkipped,
		)
	}
	if limits.MaxMemoryBytes > 0 && pixels*4 > limits.MaxMemoryBytes {
		return fmt.Errorf(
			"source image decode bytes %d exceed max memory bytes %d: %w",
			pixels*4,
			limits.MaxMemoryBytes,
			ErrVariantSkipped,
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
	seen := make(map[int]struct{}, variants.Len()+1)
	widths := make([]int, 0, variants.Len()+1)
	variants.Range(func(_ int, variant imageVariantGenerateRequest) bool {
		width := normalizedImageOutputWidth(sourceWidth, variant.TargetWidth)
		if _, ok := seen[width]; ok {
			return true
		}
		seen[width] = struct{}{}
		widths = append(widths, width)
		return true
	})
	sort.Slice(widths, func(left, right int) bool {
		return widths[left] > widths[right]
	})
	return widths
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
