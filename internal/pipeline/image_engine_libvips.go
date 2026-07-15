//go:build spack_libvips

package pipeline

import (
	"log/slog"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/davidbyttow/govips/v2/vips"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/media"
	"github.com/samber/oops"
)

type libvipsImageEngine struct {
	logger     *slog.Logger
	telemetry  imageEngineTelemetry
	memory     *imageMemoryBudget
	startupErr error
}

type libvipsSourceImage struct {
	ref    *vips.ImageRef
	width  int
	height int
	bytes  int64
}

func newLibvipsImageEngine(
	cfg *config.Image,
	logger *slog.Logger,
	telemetry imageEngineTelemetry,
) imageEngine {
	err := startLibvips()
	engine := libvipsImageEngine{
		logger:     logger.With(slog.String("engine", "libvips")),
		telemetry:  telemetry,
		memory:     newImageMemoryBudget(imageEngineMemoryBudgetLimit(cfg)),
		startupErr: err,
	}
	if err != nil {
		engine.logger.Error("Libvips image engine unavailable", slog.String("err", err.Error()))
		return engine
	}
	engine.logger.Info("Image engine configured", slog.String("engine", "libvips"))
	return engine
}

func (libvipsImageEngine) Name() string {
	return "libvips"
}

func (engine libvipsImageEngine) SupportsSourceMediaType(mediaType string) bool {
	if engine.startupErr != nil {
		return false
	}
	switch media.ImageFormat(mediaType) {
	case "jpeg", "png", "webp", "avif":
		return true
	default:
		return false
	}
}

func (engine libvipsImageEngine) SupportedTargetFormats() *cxlist.List[string] {
	if engine.startupErr != nil {
		return cxlist.NewList[string]()
	}
	return cxlist.NewList[string]("jpeg", "png", "webp", "avif")
}

func (engine libvipsImageEngine) Generate(request imageGenerateRequest) (imageGenerateResult, error) {
	results, err := engine.GenerateBatch(imageGenerateBatchRequest{
		Context:         request.Context,
		SourcePath:      request.SourcePath,
		SourceBytes:     request.SourceBytes,
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

func (engine libvipsImageEngine) GenerateBatch(
	request imageGenerateBatchRequest,
) (*cxlist.List[imageGenerateResult], error) {
	logger := engine.log()
	startedAt := time.Now()
	batchAttrs := imageBatchLogAttrs(request)
	if err := engine.startupErr; err != nil {
		engine.recordLibvipsBatchError(logger, startedAt, err, batchAttrs)
		return nil, oops.Wrapf(err, "start libvips image engine")
	}
	if request.Variants == nil || request.Variants.IsEmpty() {
		logger.Debug("Libvips image generation skipped", mergeImageLogAttrs(
			batchAttrs,
			slog.String("reason", "no_variants"),
		)...)
		engine.telemetry.recordOperation(engine.Name(), "batch", "skipped", startedAt)
		engine.telemetry.recordSkip(engine.Name(), "no_variants")
		return cxlist.NewList[imageGenerateResult](), nil
	}

	logger.Debug("Libvips image generation started", batchAttrs...)
	source, releaseMemory, err := engine.loadSourceImage(request)
	if err != nil {
		engine.recordLibvipsBatchError(logger, startedAt, err, batchAttrs)
		return nil, err
	}
	defer releaseMemory()
	defer source.ref.Close()

	results, err := engine.generateLibvipsVariants(logger, request, source, batchAttrs)
	if err != nil {
		engine.telemetry.recordOperation(engine.Name(), "batch", imageEngineResult(err), startedAt)
		return nil, err
	}
	logCompletedLibvipsBatch(logger, batchAttrs, source, results.Len())
	engine.telemetry.recordOperation(engine.Name(), "batch", "ok", startedAt)
	return results, nil
}

func (engine libvipsImageEngine) log() *slog.Logger {
	if engine.logger == nil {
		return normalizeImageEngineLogger(nil).With(slog.String("engine", "libvips"))
	}
	return engine.logger
}

func (engine libvipsImageEngine) recordLibvipsBatchError(
	logger *slog.Logger,
	startedAt time.Time,
	err error,
	attrs []any,
) {
	logImageGenerationError(logger, "Libvips image generation failed", err, attrs)
	engine.telemetry.recordOperation(engine.Name(), "batch", imageEngineResult(err), startedAt)
	engine.telemetry.recordSkip(engine.Name(), imageEngineSkipReason(err))
}
