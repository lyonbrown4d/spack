package pipeline

import (
	"context"
	"fmt"
	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/artifact"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/media"
	"github.com/samber/oops"
	"golang.org/x/sync/semaphore"
	"strconv"
	"time"
)

type imageStage struct {
	cfg         *config.Image
	engine      imageEngine
	store       artifact.Store
	catalog     catalog.Catalog
	sourceSlots *semaphore.Weighted
}

func newImageStage(cfg *config.Image, engine imageEngine, store artifact.Store, cat catalog.Catalog) *imageStage {
	stage := &imageStage{
		cfg:     cfg,
		engine:  engine,
		store:   store,
		catalog: cat,
	}
	if cfg != nil && cfg.MaxConcurrentSources > 0 {
		stage.sourceSlots = semaphore.NewWeighted(int64(cfg.MaxConcurrentSources))
	}
	return stage
}

func (s *imageStage) Name() string {
	return "image"
}

func (s *imageStage) Plan(asset *catalog.Asset, request Request) *cxlist.List[Task] {
	if !s.cfg.Enable || !isResizableImage(s.engine, asset) {
		return nil
	}

	formats := s.planFormats(asset, request)
	widths := s.planWidths(asset, request, formats)
	if widths.IsEmpty() || formats.IsEmpty() {
		return cxlist.NewList[Task]()
	}

	return s.planTasks(asset, formats, widths)
}

func (s *imageStage) Execute(task Task, asset *catalog.Asset) (*catalog.Variant, error) {
	variants, err := s.ExecuteBatch(task, asset)
	if err != nil {
		return nil, err
	}
	variant, ok := variants.Get(0)
	if !ok {
		return nil, ErrVariantSkipped
	}
	return variant, nil
}

func (s *imageStage) ExecuteBatch(task Task, asset *catalog.Asset) (*cxlist.List[*catalog.Variant], error) {
	requests, err := s.imageGenerateRequests(task, asset)
	if err != nil {
		return nil, err
	}
	if requests.IsEmpty() {
		return nil, ErrVariantSkipped
	}

	release, err := s.acquireSourceSlot()
	if err != nil {
		return nil, err
	}
	defer release()

	results, err := s.engine.GenerateBatch(imageGenerateBatchRequest{
		SourcePath:      asset.FullPath,
		SourceMediaType: asset.MediaType,
		Variants:        requests,
		Encode: imageEncodeOptions{
			JPEGQuality: s.cfg.JPEGQuality,
		},
		Limits: imageGenerateLimitsFromConfig(s.cfg),
	})
	if err != nil {
		return nil, fmt.Errorf("generate image artifact: %w", err)
	}

	return s.writeImageVariants(asset, results)
}

func (s *imageStage) writeImageVariants(
	asset *catalog.Asset,
	results *cxlist.List[imageGenerateResult],
) (*cxlist.List[*catalog.Variant], error) {
	variants := cxlist.NewList[*catalog.Variant]()
	var writeErr error
	results.Range(func(_ int, result imageGenerateResult) bool {
		if result.Width <= 0 || s.shouldSkipImageArtifact(asset, result) {
			return true
		}
		variant, err := s.writeImageVariant(asset, result)
		if err != nil {
			writeErr = err
			return false
		}
		variants.Add(variant)
		return true
	})
	if writeErr != nil {
		return nil, writeErr
	}
	if variants.IsEmpty() {
		return nil, ErrVariantSkipped
	}
	return variants, nil
}

func (s *imageStage) imageGenerateRequests(
	task Task,
	asset *catalog.Asset,
) (*cxlist.List[imageVariantGenerateRequest], error) {
	if task.ImageVariants != nil && !task.ImageVariants.IsEmpty() {
		return cxlist.FilterMapList[ImageVariantTask, imageVariantGenerateRequest](
			task.ImageVariants,
			func(_ int, variant ImageVariantTask) (imageVariantGenerateRequest, bool) {
				targetFormat, err := resolveImageVariantTargetFormat(variant, asset)
				if err != nil {
					return imageVariantGenerateRequest{}, false
				}
				return imageVariantGenerateRequest{
					TargetFormat: targetFormat,
					TargetWidth:  variant.Width,
				}, true
			},
		), nil
	}

	targetFormat, err := resolveTargetFormat(task, asset)
	if err != nil {
		return nil, err
	}
	return cxlist.NewList(imageVariantGenerateRequest{
		TargetFormat: targetFormat,
		TargetWidth:  task.Width,
	}), nil
}

func (s *imageStage) writeImageVariant(asset *catalog.Asset, result imageGenerateResult) (*catalog.Variant, error) {
	targetPath := s.store.PathFor(asset.Path, asset.SourceHash, "image", imageVariantSuffix(result.Width, result.TargetFormat, result.Extension))
	if err := s.store.Write(targetPath, result.Payload); err != nil {
		return nil, oops.Wrap(fmt.Errorf("write image artifact: %w", err))
	}
	sourcePixels := int64(result.SourceWidth) * int64(result.SourceHeight)
	outputBytes := int64(len(result.Payload))
	savedBytes := max(result.SourceBytes-outputBytes, 0)

	return &catalog.Variant{
		ID:           imageVariantID(asset.Path, result.Width, result.TargetFormat),
		AssetPath:    asset.Path,
		ArtifactPath: targetPath,
		Size:         outputBytes,
		MediaType:    result.MediaType,
		SourceHash:   asset.SourceHash,
		ETag:         imageVariantETag(asset.SourceHash, result.Width, result.TargetFormat),
		Format:       result.TargetFormat,
		Width:        result.Width,
		Metadata: catalog.MetadataWithModTime(cxmapping.NewMapFrom(map[string]string{
			"stage":         "image",
			"backend":       s.engine.Name(),
			"source_bytes":  strconv.FormatInt(result.SourceBytes, 10),
			"source_pixels": strconv.FormatInt(sourcePixels, 10),
			"source_width":  strconv.Itoa(result.SourceWidth),
			"source_height": strconv.Itoa(result.SourceHeight),
			"output_bytes":  strconv.FormatInt(outputBytes, 10),
			"output_width":  strconv.Itoa(result.Width),
			"output_height": strconv.Itoa(result.Height),
			"saved_bytes":   strconv.FormatInt(savedBytes, 10),
			"saving_ratio":  fmt.Sprintf("%.6f", savingRatio(result.SourceBytes, outputBytes)),
			"image_engine":  s.engine.Name(),
			"target_format": result.TargetFormat,
		}), time.Now()),
	}, nil
}

func resolveImageVariantTargetFormat(variant ImageVariantTask, asset *catalog.Asset) (string, error) {
	return resolveTargetFormat(Task{Format: variant.Format, Width: variant.Width}, asset)
}

func (s *imageStage) acquireSourceSlot() (func(), error) {
	if s.sourceSlots == nil {
		return func() {}, nil
	}
	if err := s.sourceSlots.Acquire(context.Background(), 1); err != nil {
		return func() {}, fmt.Errorf("acquire image source slot: %w", err)
	}
	return func() {
		s.sourceSlots.Release(1)
	}, nil
}

func savingRatio(sourceBytes, outputBytes int64) float64 {
	if sourceBytes <= 0 || outputBytes >= sourceBytes {
		return 0
	}
	return float64(sourceBytes-outputBytes) / float64(sourceBytes)
}

func isResizableImage(engine imageEngine, asset *catalog.Asset) bool {
	if engine == nil || asset == nil {
		return false
	}
	if !media.IsImageMediaType(asset.MediaType) {
		return false
	}
	return engine.SupportsSourceMediaType(asset.MediaType)
}

func normalizeImageFormats(formats *cxlist.List[string]) *cxlist.List[string] {
	return media.NormalizeImageFormats(formats)
}

func imageVariantSuffix(width int, format, ext string) string {
	parts := cxlist.NewList[string]()
	if width > 0 {
		parts.Add(fmt.Sprintf("w%d", width))
	}
	if format != "" {
		parts.Add("f" + format)
	}
	if parts.IsEmpty() {
		return ext
	}
	return "." + parts.Join(".") + ext
}

func imageVariantID(assetPath string, width int, format string) string {
	parts := cxlist.NewList(assetPath)
	if width > 0 {
		parts.Add(fmt.Sprintf("width=%d", width))
	}
	if format != "" {
		parts.Add("format=" + format)
	}
	return parts.Join("|")
}

func imageVariantETag(sourceHash string, width int, format string) string {
	parts := cxlist.NewList(sourceHash)
	if width > 0 {
		parts.Add(fmt.Sprintf("w%d", width))
	}
	if format != "" {
		parts.Add(format)
	}
	return "\"" + parts.Join("-") + "\""
}
