//go:build libvips && cgo

package pipeline

import (
	"fmt"
	"log/slog"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/davidbyttow/govips/v2/vips"
	"github.com/lyonbrown4d/spack/internal/media"
)

func (engine libvipsImageEngine) exportLibvipsVariants(
	logger *slog.Logger,
	request imageGenerateBatchRequest,
	source libvipsSourceImage,
	pyramid map[int]*vips.ImageRef,
	batchAttrs []any,
) (*cxlist.List[imageGenerateResult], error) {
	results := cxlist.NewListWithCapacity[imageGenerateResult](request.Variants.Len())
	var generateErr error
	request.Variants.Range(func(_ int, variant imageVariantGenerateRequest) bool {
		result, err := engine.exportLibvipsVariant(logger, request, source, pyramid, batchAttrs, variant)
		if err != nil {
			generateErr = err
			return false
		}
		results.Add(result)
		return true
	})
	if generateErr != nil {
		return nil, generateErr
	}
	return results, nil
}

func (engine libvipsImageEngine) exportLibvipsVariant(
	logger *slog.Logger,
	request imageGenerateBatchRequest,
	source libvipsSourceImage,
	pyramid map[int]*vips.ImageRef,
	batchAttrs []any,
	variant imageVariantGenerateRequest,
) (imageGenerateResult, error) {
	width := normalizedBuiltinOutputWidth(source.width, variant.TargetWidth)
	variantAttrs := mergeBuiltinImageLogAttrs(batchAttrs, builtinVariantLogAttrs(variant, width)...)
	output, ok := pyramid[width]
	if !ok {
		err := fmt.Errorf("missing libvips image pyramid width %d", width)
		logLibvipsImageGenerationError(logger, "Libvips image variant failed", err, variantAttrs)
		engine.telemetry.recordOperation(engine.Name(), "encode", "error", time.Now())
		return imageGenerateResult{}, err
	}

	encodeStartedAt := time.Now()
	payload, ext, mediaType, err := exportLibvipsImage(output, variant.TargetFormat, request.Encode)
	engine.telemetry.recordOperation(engine.Name(), "encode", imageEngineResult(err), encodeStartedAt)
	if err != nil {
		logLibvipsImageGenerationError(logger, "Libvips image variant failed", err, variantAttrs)
		return imageGenerateResult{}, err
	}

	result := libvipsGenerateResult(source, output, variant, payload, ext, mediaType)
	engine.recordGeneratedLibvipsVariant(result)
	logGeneratedLibvipsImageVariant(logger, variantAttrs, result)
	return result, nil
}

func exportLibvipsImage(
	image *vips.ImageRef,
	format string,
	opts imageEncodeOptions,
) ([]byte, string, string, error) {
	descriptor, ok := media.LookupImageDescriptor(media.NormalizeImageFormat(format))
	if !ok {
		return nil, "", "", fmt.Errorf("unsupported image format: %s", format)
	}

	payload, err := exportLibvipsImagePayload(image, descriptor.Name, opts)
	if err != nil {
		return nil, "", "", err
	}
	return payload, descriptor.Extension, descriptor.MediaType, nil
}

func exportLibvipsImagePayload(image *vips.ImageRef, format string, opts imageEncodeOptions) ([]byte, error) {
	switch format {
	case "jpeg":
		params := vips.NewJpegExportParams()
		params.Quality = clampJPEGQuality(opts.JPEGQuality)
		params.StripMetadata = true
		payload, _, err := image.ExportJpeg(params)
		return payload, wrapLibvipsExportError("jpeg", err)
	case "png":
		params := vips.NewPngExportParams()
		params.StripMetadata = true
		payload, _, err := image.ExportPng(params)
		return payload, wrapLibvipsExportError("png", err)
	case "webp":
		params := vips.NewWebpExportParams()
		params.Quality = clampJPEGQuality(opts.JPEGQuality)
		params.StripMetadata = true
		payload, _, err := image.ExportWebp(params)
		return payload, wrapLibvipsExportError("webp", err)
	case "avif":
		params := vips.NewAvifExportParams()
		params.Quality = clampJPEGQuality(opts.JPEGQuality)
		params.Effort = 4
		params.StripMetadata = true
		payload, _, err := image.ExportAvif(params)
		return payload, wrapLibvipsExportError("avif", err)
	default:
		return nil, fmt.Errorf("libvips engine does not support %s output", format)
	}
}

func wrapLibvipsExportError(format string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("export %s image with libvips: %w", format, err)
}

func (engine libvipsImageEngine) recordGeneratedLibvipsVariant(result imageGenerateResult) {
	engine.telemetry.recordVariant(
		engine.Name(),
		result.TargetFormat,
		int64(len(result.Payload)),
		max(result.SourceBytes-int64(len(result.Payload)), 0),
		savingRatio(result.SourceBytes, int64(len(result.Payload))),
	)
}

func logGeneratedLibvipsImageVariant(
	logger *slog.Logger,
	variantAttrs []any,
	result imageGenerateResult,
) {
	logger.Debug("Libvips image variant generated",
		mergeBuiltinImageLogAttrs(variantAttrs,
			slog.Int("output_width", result.Width),
			slog.Int("output_height", result.Height),
			slog.Int("output_bytes", len(result.Payload)),
		)...,
	)
}
