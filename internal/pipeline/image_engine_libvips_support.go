//go:build spack_libvips

package pipeline

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/davidbyttow/govips/v2/vips"
	"github.com/lyonbrown4d/spack/internal/media"
)

var (
	libvipsStartupOnce sync.Once
	libvipsStartupErr  error
)

func startLibvips() error {
	libvipsStartupOnce.Do(func() {
		if err := vips.Startup(nil); err != nil {
			libvipsStartupErr = fmt.Errorf("start libvips: %w", err)
		}
	})
	return libvipsStartupErr
}

func (engine libvipsImageEngine) loadSourceImage(
	request imageGenerateBatchRequest,
) (_ libvipsSourceImage, _ func(), err error) {
	sourceBytes, err := imageSourceBytes(request.SourcePath, request.Limits)
	if err != nil {
		return libvipsSourceImage{}, noopMemoryRelease, err
	}

	decodeStartedAt := time.Now()
	ref, err := vips.NewImageFromFile(request.SourcePath)
	if err == nil {
		err = ref.AutoRotate()
	}
	engine.telemetry.recordOperation(engine.Name(), "decode", imageEngineResult(err), decodeStartedAt)
	if err != nil {
		closeLibvipsImage(ref)
		return libvipsSourceImage{}, noopMemoryRelease, fmt.Errorf("decode source image with libvips: %w", err)
	}

	source := libvipsSourceImage{
		ref:    ref,
		width:  ref.Width(),
		height: ref.Height(),
		bytes:  sourceBytes,
	}
	if validationErr := validateImageSourceLimits(imageSourceSnapshot{
		width:  source.width,
		height: source.height,
		bytes:  source.bytes,
	}, request.Limits); validationErr != nil {
		closeLibvipsImage(ref)
		return libvipsSourceImage{}, noopMemoryRelease, validationErr
	}

	releaseMemory, err := engine.acquireSourceMemory(request, source)
	if err != nil {
		closeLibvipsImage(ref)
		return libvipsSourceImage{}, noopMemoryRelease, err
	}
	engine.telemetry.recordSource(engine.Name(), source.bytes, source.width, source.height)
	return source, releaseMemory, nil
}

func (engine libvipsImageEngine) acquireSourceMemory(
	request imageGenerateBatchRequest,
	source libvipsSourceImage,
) (func(), error) {
	waitStartedAt := time.Now()
	memoryBytes := estimateImageBatchMemoryBytes(source.width, source.height, request.Variants)
	releaseMemory, err := engine.memory.Acquire(memoryBytes)
	engine.telemetry.recordOperation(engine.Name(), "memory_wait", imageEngineResult(err), waitStartedAt)
	return releaseMemory, err
}

func (engine libvipsImageEngine) generateLibvipsVariants(
	logger *slog.Logger,
	request imageGenerateBatchRequest,
	source libvipsSourceImage,
	batchAttrs []any,
) (*cxlist.List[imageGenerateResult], error) {
	resizeStartedAt := time.Now()
	pyramid, err := buildLibvipsPyramid(source, request.Variants)
	engine.telemetry.recordOperation(engine.Name(), "resize", imageEngineResult(err), resizeStartedAt)
	if err != nil {
		return nil, err
	}
	defer closeLibvipsPyramid(pyramid, source.ref)
	return engine.exportLibvipsVariants(logger, request, source, pyramid, batchAttrs)
}

func buildLibvipsPyramid(
	source libvipsSourceImage,
	variants *cxlist.List[imageVariantGenerateRequest],
) (map[int]*vips.ImageRef, error) {
	widths := uniqueImageOutputWidths(source.width, variants)
	pyramid := map[int]*vips.ImageRef{source.width: source.ref}
	current := source.ref
	currentWidth := source.width
	for _, width := range widths {
		if width == source.width {
			continue
		}
		if width > currentWidth {
			pyramid[width] = source.ref
			continue
		}
		resized, err := resizeLibvipsImage(current, currentWidth, width)
		if err != nil {
			closeLibvipsPyramid(pyramid, source.ref)
			return nil, err
		}
		pyramid[width] = resized
		current = resized
		currentWidth = width
	}
	return pyramid, nil
}

func resizeLibvipsImage(source *vips.ImageRef, sourceWidth, width int) (*vips.ImageRef, error) {
	resized, err := source.Copy()
	if err != nil {
		return nil, fmt.Errorf("copy libvips image: %w", err)
	}
	if err := resized.Resize(float64(width)/float64(sourceWidth), vips.KernelLanczos3); err != nil {
		resized.Close()
		return nil, fmt.Errorf("resize libvips image: %w", err)
	}
	return resized, nil
}

func closeLibvipsPyramid(pyramid map[int]*vips.ImageRef, source *vips.ImageRef) {
	closed := map[*vips.ImageRef]struct{}{source: {}}
	for _, image := range pyramid {
		if image == nil {
			continue
		}
		if _, ok := closed[image]; ok {
			continue
		}
		closed[image] = struct{}{}
		image.Close()
	}
}

func closeLibvipsImage(image *vips.ImageRef) {
	if image != nil {
		image.Close()
	}
}

func libvipsGenerateResult(
	source libvipsSourceImage,
	output *vips.ImageRef,
	variant imageVariantGenerateRequest,
	payload []byte,
	ext string,
	mediaType string,
) imageGenerateResult {
	return imageGenerateResult{
		Payload:      payload,
		Width:        output.Width(),
		Height:       output.Height(),
		SourceWidth:  source.width,
		SourceHeight: source.height,
		SourceBytes:  source.bytes,
		TargetFormat: media.NormalizeImageFormat(variant.TargetFormat),
		MediaType:    mediaType,
		Extension:    ext,
	}
}
