package pipeline

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"sort"

	"github.com/anthonynsimon/bild/transform"
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/media"
)

func builtinSourceBytes(path string, limits imageGenerateLimits) (int64, error) {
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

func validateBuiltinSourceImage(source builtinSourceImage, limits imageGenerateLimits) error {
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
	return validateBuiltinPixelLimits(source, limits)
}

func validateBuiltinPixelLimits(source builtinSourceImage, limits imageGenerateLimits) error {
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

func buildBuiltinPyramid(
	source builtinSourceImage,
	variants *cxlist.List[imageVariantGenerateRequest],
) map[int]builtinPyramidImage {
	widths := uniqueBuiltinOutputWidths(source.width, variants)
	pyramid := map[int]builtinPyramidImage{
		source.width: {
			Image:  source.Image,
			width:  source.width,
			height: source.height,
		},
	}

	current := pyramid[source.width]
	for _, width := range widths {
		if width == source.width {
			continue
		}
		if width > current.width {
			pyramid[width] = pyramid[source.width]
			continue
		}
		resized := resizeBuiltinImage(current.Image, current.width, current.height, width)
		pyramid[width] = resized
		current = resized
	}
	return pyramid
}

func uniqueBuiltinOutputWidths(sourceWidth int, variants *cxlist.List[imageVariantGenerateRequest]) []int {
	seen := make(map[int]struct{}, variants.Len()+1)
	widths := make([]int, 0, variants.Len()+1)
	variants.Range(func(_ int, variant imageVariantGenerateRequest) bool {
		width := normalizedBuiltinOutputWidth(sourceWidth, variant.TargetWidth)
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

func normalizedBuiltinOutputWidth(sourceWidth, targetWidth int) int {
	if targetWidth <= 0 || targetWidth >= sourceWidth {
		return sourceWidth
	}
	return targetWidth
}

func resizeBuiltinImage(srcImage image.Image, srcWidth, srcHeight, targetWidth int) builtinPyramidImage {
	outputWidth := normalizedBuiltinOutputWidth(srcWidth, targetWidth)
	if outputWidth == srcWidth {
		return builtinPyramidImage{
			Image:  srcImage,
			width:  srcWidth,
			height: srcHeight,
		}
	}

	targetHeight := max(1, srcHeight*outputWidth/srcWidth)
	return builtinPyramidImage{
		Image:  transform.Resize(srcImage, outputWidth, targetHeight, transform.CatmullRom),
		width:  outputWidth,
		height: targetHeight,
	}
}

func encodeBuiltinImage(img image.Image, format string, opts imageEncodeOptions) ([]byte, string, string, error) {
	descriptor, ok := media.LookupImageDescriptor(media.NormalizeImageFormat(format))
	if !ok {
		return nil, "", "", fmt.Errorf("unsupported image format: %s", format)
	}

	var buffer bytes.Buffer
	switch descriptor.Name {
	case "jpeg":
		if err := jpeg.Encode(&buffer, img, &jpeg.Options{Quality: clampJPEGQuality(opts.JPEGQuality)}); err != nil {
			return nil, "", "", fmt.Errorf("encode jpeg image: %w", err)
		}
	case "png":
		if err := png.Encode(&buffer, img); err != nil {
			return nil, "", "", fmt.Errorf("encode png image: %w", err)
		}
	default:
		return nil, "", "", fmt.Errorf("builtin engine does not support %s output", descriptor.Name)
	}

	return buffer.Bytes(), descriptor.Extension, descriptor.MediaType, nil
}

func clampJPEGQuality(quality int) int {
	if quality < 1 {
		return 1
	}
	if quality > 100 {
		return 100
	}
	return quality
}
