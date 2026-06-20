package pipeline

import (
	"fmt"
	"log/slog"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/media"
)

func (engine builtinImageEngine) generateBatchVariants(
	logger *slog.Logger,
	request imageGenerateBatchRequest,
	source builtinSourceImage,
	pyramid map[int]builtinPyramidImage,
	batchAttrs []any,
) (*cxlist.List[imageGenerateResult], error) {
	results := cxlist.NewListWithCapacity[imageGenerateResult](request.Variants.Len())
	var generateErr error
	request.Variants.Range(func(_ int, variant imageVariantGenerateRequest) bool {
		result, err := engine.generateBatchVariant(logger, request, source, pyramid, batchAttrs, variant)
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

func (engine builtinImageEngine) generateBatchVariant(
	logger *slog.Logger,
	request imageGenerateBatchRequest,
	source builtinSourceImage,
	pyramid map[int]builtinPyramidImage,
	batchAttrs []any,
	variant imageVariantGenerateRequest,
) (imageGenerateResult, error) {
	width := normalizedBuiltinOutputWidth(source.width, variant.TargetWidth)
	variantAttrs := mergeBuiltinImageLogAttrs(batchAttrs, builtinVariantLogAttrs(variant, width)...)
	output, ok := pyramid[width]
	if !ok {
		err := fmt.Errorf("missing image pyramid width %d", width)
		logBuiltinImageGenerationError(logger, "Builtin image variant failed", err, variantAttrs)
		engine.telemetry.recordOperation(engine.Name(), "encode", "error", time.Now())
		return imageGenerateResult{}, err
	}

	encodeStartedAt := time.Now()
	payload, ext, mediaType, err := encodeBuiltinImage(output.Image, variant.TargetFormat, request.Encode)
	engine.telemetry.recordOperation(engine.Name(), "encode", imageEngineResult(err), encodeStartedAt)
	if err != nil {
		logBuiltinImageGenerationError(logger, "Builtin image variant failed", err, variantAttrs)
		return imageGenerateResult{}, err
	}

	result := builtinImageGenerateResult(source, output, variant, payload, ext, mediaType)
	engine.recordGeneratedImageVariant(result)
	logGeneratedBuiltinImageVariant(logger, variantAttrs, output, payload)
	return result, nil
}

func builtinImageGenerateResult(
	source builtinSourceImage,
	output builtinPyramidImage,
	variant imageVariantGenerateRequest,
	payload []byte,
	ext string,
	mediaType string,
) imageGenerateResult {
	return imageGenerateResult{
		Payload:      payload,
		Width:        output.width,
		Height:       output.height,
		SourceWidth:  source.width,
		SourceHeight: source.height,
		SourceBytes:  source.bytes,
		TargetFormat: media.NormalizeImageFormat(variant.TargetFormat),
		MediaType:    mediaType,
		Extension:    ext,
	}
}

func (engine builtinImageEngine) recordGeneratedImageVariant(result imageGenerateResult) {
	engine.telemetry.recordVariant(
		engine.Name(),
		result.TargetFormat,
		int64(len(result.Payload)),
		max(result.SourceBytes-int64(len(result.Payload)), 0),
		savingRatio(result.SourceBytes, int64(len(result.Payload))),
	)
}

func logGeneratedBuiltinImageVariant(
	logger *slog.Logger,
	variantAttrs []any,
	output builtinPyramidImage,
	payload []byte,
) {
	logger.Debug("Builtin image variant generated",
		mergeBuiltinImageLogAttrs(variantAttrs,
			slog.Int("output_width", output.width),
			slog.Int("output_height", output.height),
			slog.Int("output_bytes", len(payload)),
		)...,
	)
}
