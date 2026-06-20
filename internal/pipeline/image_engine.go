package pipeline

import (
	"fmt"
	"image"
	"log/slog"
	"os"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/media"
)

type imageEncodeOptions struct {
	JPEGQuality int
}

type imageGenerateLimits struct {
	MaxSourceBytes  int64
	MaxSourcePixels int64
	MaxWidth        int
	MaxHeight       int
	MaxMemoryBytes  int64
}

type imageVariantGenerateRequest struct {
	TargetFormat string
	TargetWidth  int
}

type imageGenerateRequest struct {
	SourcePath      string
	SourceMediaType string
	TargetFormat    string
	TargetWidth     int
	Encode          imageEncodeOptions
	Limits          imageGenerateLimits
}

type imageGenerateBatchRequest struct {
	SourcePath      string
	SourceMediaType string
	Variants        *cxlist.List[imageVariantGenerateRequest]
	Encode          imageEncodeOptions
	Limits          imageGenerateLimits
}

type imageGenerateResult struct {
	Payload      []byte
	Width        int
	Height       int
	SourceWidth  int
	SourceHeight int
	SourceBytes  int64
	TargetFormat string
	MediaType    string
	Extension    string
}

type imageEngine interface {
	Name() string
	SupportsSourceMediaType(mediaType string) bool
	SupportedTargetFormats() *cxlist.List[string]
	Generate(request imageGenerateRequest) (imageGenerateResult, error)
	GenerateBatch(request imageGenerateBatchRequest) (*cxlist.List[imageGenerateResult], error)
}

func normalizeImageEngineLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default().WithGroup("image_engine")
	}
	return logger.WithGroup("image_engine")
}

type builtinImageEngine struct {
	logger    *slog.Logger
	telemetry imageEngineTelemetry
	memory    *imageMemoryBudget
}

type builtinSourceImage struct {
	image.Image
	width  int
	height int
	bytes  int64
}

type builtinPyramidImage struct {
	image.Image
	width  int
	height int
}

func (builtinImageEngine) Name() string {
	return "builtin"
}

func (builtinImageEngine) SupportsSourceMediaType(mediaType string) bool {
	switch media.ImageFormat(mediaType) {
	case "jpeg", "png":
		return true
	default:
		return false
	}
}

func (builtinImageEngine) SupportedTargetFormats() *cxlist.List[string] {
	return cxlist.NewList[string]("jpeg", "png")
}

func (engine builtinImageEngine) Generate(request imageGenerateRequest) (imageGenerateResult, error) {
	results, err := engine.GenerateBatch(imageGenerateBatchRequest{
		SourcePath:      request.SourcePath,
		SourceMediaType: request.SourceMediaType,
		Variants: cxlist.NewList(imageVariantGenerateRequest{
			TargetFormat: request.TargetFormat,
			TargetWidth:  request.TargetWidth,
		}),
		Encode: request.Encode,
		Limits: request.Limits,
	})
	if err != nil {
		return imageGenerateResult{}, err
	}
	result, ok := results.Get(0)
	if !ok {
		return imageGenerateResult{}, ErrVariantSkipped
	}
	return result, nil
}

func (engine builtinImageEngine) GenerateBatch(request imageGenerateBatchRequest) (*cxlist.List[imageGenerateResult], error) {
	logger := engine.log()
	startedAt := time.Now()
	batchAttrs := builtinImageBatchLogAttrs(request)
	if request.Variants == nil || request.Variants.IsEmpty() {
		logger.Debug("Builtin image generation skipped", mergeBuiltinImageLogAttrs(
			batchAttrs,
			slog.String("reason", "no_variants"),
		)...)
		engine.telemetry.recordOperation(engine.Name(), "batch", "skipped", startedAt)
		engine.telemetry.recordSkip(engine.Name(), "no_variants")
		return cxlist.NewList[imageGenerateResult](), nil
	}

	logger.Debug("Builtin image generation started", batchAttrs...)
	source, releaseMemory, err := engine.loadSourceImage(request)
	if err != nil {
		logBuiltinImageGenerationError(logger, "Builtin image source rejected", err, batchAttrs)
		engine.telemetry.recordOperation(engine.Name(), "batch", imageEngineResult(err), startedAt)
		engine.telemetry.recordSkip(engine.Name(), imageEngineSkipReason(err))
		return nil, err
	}
	defer releaseMemory()
	logger.Debug("Builtin image source decoded", mergeBuiltinImageLogAttrs(batchAttrs, builtinSourceLogAttrs(source)...)...)
	engine.telemetry.recordSource(engine.Name(), source.bytes, source.width, source.height)
	resizeStartedAt := time.Now()
	pyramid := buildBuiltinPyramid(source, request.Variants)
	engine.telemetry.recordOperation(engine.Name(), "resize", "ok", resizeStartedAt)

	results, err := engine.generateBatchVariants(logger, request, source, pyramid, batchAttrs)
	if err != nil {
		engine.telemetry.recordOperation(engine.Name(), "batch", imageEngineResult(err), startedAt)
		return nil, err
	}
	logger.Debug("Builtin image generation completed",
		mergeBuiltinImageLogAttrs(batchAttrs,
			slog.Int("generated_variants", results.Len()),
			slog.Int("source_width", source.width),
			slog.Int("source_height", source.height),
			slog.Int64("source_bytes", source.bytes),
		)...,
	)
	engine.telemetry.recordOperation(engine.Name(), "batch", "ok", startedAt)
	return results, nil
}

func (engine builtinImageEngine) loadSourceImage(
	request imageGenerateBatchRequest,
) (_ builtinSourceImage, _ func(), err error) {
	source, memoryBytes, err := readBuiltinSourceImageConfig(request.SourcePath, request.Limits, request.Variants)
	if err != nil {
		return builtinSourceImage{}, noopMemoryRelease, err
	}
	waitStartedAt := time.Now()
	releaseMemory, err := engine.memory.Acquire(memoryBytes)
	engine.telemetry.recordOperation(engine.Name(), "memory_wait", imageEngineResult(err), waitStartedAt)
	if err != nil {
		return builtinSourceImage{}, noopMemoryRelease, err
	}

	decodeStartedAt := time.Now()
	source.Image, err = decodeBuiltinSourceImage(request.SourcePath)
	engine.telemetry.recordOperation(engine.Name(), "decode", imageEngineResult(err), decodeStartedAt)
	if err != nil {
		releaseMemory()
		return builtinSourceImage{}, noopMemoryRelease, err
	}
	return source, releaseMemory, nil
}

func decodeBuiltinSourceImage(path string) (_ image.Image, err error) {
	// #nosec G304 -- path comes from scanned assets rooted under configured sources.
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open source image: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close source image: %w", closeErr)
		}
	}()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("decode source image: %w", err)
	}
	return img, nil
}
