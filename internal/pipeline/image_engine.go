package pipeline

import (
	"fmt"
	"image"
	"log/slog"
	"os"
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/config"
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

func newImageEngine(cfg *config.Image, logger *slog.Logger) imageEngine {
	baseLogger := normalizeImageEngineLogger(logger)
	engineName := configuredImageEngineName(cfg)
	if engineName == "builtin" {
		baseLogger.Info("Image engine configured", slog.String("engine", "builtin"))
		return builtinImageEngine{logger: baseLogger.With(slog.String("engine", "builtin"))}
	}
	baseLogger.Warn("Unknown image engine configured; falling back to builtin",
		slog.String("configured_engine", engineName),
		slog.String("engine", "builtin"),
	)
	return builtinImageEngine{logger: baseLogger.With(slog.String("engine", "builtin"))}
}

func configuredImageEngineName(cfg *config.Image) string {
	if cfg == nil {
		return "builtin"
	}
	name := strings.TrimSpace(cfg.Engine)
	if name == "" {
		return "builtin"
	}
	return name
}

func normalizeImageEngineLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default().WithGroup("image_engine")
	}
	return logger.WithGroup("image_engine")
}

type builtinImageEngine struct {
	logger *slog.Logger
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
	batchAttrs := builtinImageBatchLogAttrs(request)
	if request.Variants == nil || request.Variants.IsEmpty() {
		logger.Debug("Builtin image generation skipped", mergeBuiltinImageLogAttrs(
			batchAttrs,
			slog.String("reason", "no_variants"),
		)...)
		return cxlist.NewList[imageGenerateResult](), nil
	}

	logger.Debug("Builtin image generation started", batchAttrs...)
	source, err := loadBuiltinSourceImage(request.SourcePath, request.Limits)
	if err != nil {
		logBuiltinImageGenerationError(logger, "Builtin image source rejected", err, batchAttrs)
		return nil, err
	}
	logger.Debug("Builtin image source decoded", mergeBuiltinImageLogAttrs(batchAttrs, builtinSourceLogAttrs(source)...)...)
	pyramid := buildBuiltinPyramid(source, request.Variants)

	results := cxlist.NewListWithCapacity[imageGenerateResult](request.Variants.Len())
	var generateErr error
	request.Variants.Range(func(_ int, variant imageVariantGenerateRequest) bool {
		width := normalizedBuiltinOutputWidth(source.width, variant.TargetWidth)
		variantAttrs := mergeBuiltinImageLogAttrs(batchAttrs, builtinVariantLogAttrs(variant, width)...)
		output, ok := pyramid[width]
		if !ok {
			generateErr = fmt.Errorf("missing image pyramid width %d", width)
			logBuiltinImageGenerationError(logger, "Builtin image variant failed", generateErr, variantAttrs)
			return false
		}
		payload, ext, mediaType, err := encodeBuiltinImage(output.Image, variant.TargetFormat, request.Encode)
		if err != nil {
			generateErr = err
			logBuiltinImageGenerationError(logger, "Builtin image variant failed", generateErr, variantAttrs)
			return false
		}
		results.Add(imageGenerateResult{
			Payload:      payload,
			Width:        output.width,
			Height:       output.height,
			SourceWidth:  source.width,
			SourceHeight: source.height,
			SourceBytes:  source.bytes,
			TargetFormat: media.NormalizeImageFormat(variant.TargetFormat),
			MediaType:    mediaType,
			Extension:    ext,
		})
		logger.Debug("Builtin image variant generated",
			mergeBuiltinImageLogAttrs(variantAttrs,
				slog.Int("output_width", output.width),
				slog.Int("output_height", output.height),
				slog.Int("output_bytes", len(payload)),
			)...,
		)
		return true
	})
	if generateErr != nil {
		return nil, generateErr
	}
	logger.Debug("Builtin image generation completed",
		mergeBuiltinImageLogAttrs(batchAttrs,
			slog.Int("generated_variants", results.Len()),
			slog.Int("source_width", source.width),
			slog.Int("source_height", source.height),
			slog.Int64("source_bytes", source.bytes),
		)...,
	)
	return results, nil
}

func loadBuiltinSourceImage(path string, limits imageGenerateLimits) (_ builtinSourceImage, err error) {
	sourceBytes, err := builtinSourceBytes(path, limits)
	if err != nil {
		return builtinSourceImage{}, err
	}

	// #nosec G304 -- path comes from scanned assets rooted under configured sources.
	file, err := os.Open(path)
	if err != nil {
		return builtinSourceImage{}, fmt.Errorf("open source image: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close source image: %w", closeErr)
		}
	}()

	img, _, err := image.Decode(file)
	if err != nil {
		return builtinSourceImage{}, fmt.Errorf("decode source image: %w", err)
	}

	bounds := img.Bounds()
	source := builtinSourceImage{
		Image:  img,
		width:  bounds.Dx(),
		height: bounds.Dy(),
		bytes:  sourceBytes,
	}
	if err := validateBuiltinSourceImage(source, limits); err != nil {
		return builtinSourceImage{}, err
	}
	return source, nil
}
