package pipeline

import (
	"fmt"
	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/artifact"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/media"
	"github.com/samber/oops"
	"time"
)

type imageStage struct {
	cfg     *config.Image
	engine  imageEngine
	store   artifact.Store
	catalog catalog.Catalog
}

func newImageStage(cfg *config.Image, engine imageEngine, store artifact.Store, cat catalog.Catalog) *imageStage {
	return &imageStage{
		cfg:     cfg,
		engine:  engine,
		store:   store,
		catalog: cat,
	}
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
		return nil
	}

	return s.planTasks(asset, formats, widths)
}

func (s *imageStage) Execute(task Task, asset *catalog.Asset) (*catalog.Variant, error) {
	targetFormat, err := resolveTargetFormat(task, asset)
	if err != nil {
		return nil, err
	}

	result, err := s.engine.Generate(imageGenerateRequest{
		SourcePath:      asset.FullPath,
		SourceMediaType: asset.MediaType,
		TargetFormat:    targetFormat,
		TargetWidth:     task.Width,
		Encode: imageEncodeOptions{
			JPEGQuality: s.cfg.JPEGQuality,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("generate image artifact: %w", err)
	}
	if result.Width <= 0 {
		return nil, ErrVariantSkipped
	}
	if shouldSkipImageArtifact(asset, result) {
		return nil, ErrVariantSkipped
	}

	targetPath := s.store.PathFor(asset.Path, asset.SourceHash, "image", imageVariantSuffix(result.Width, targetFormat, result.Extension))
	if err := s.store.Write(targetPath, result.Payload); err != nil {
		return nil, oops.Wrap(fmt.Errorf("write image artifact: %w", err))
	}

	return &catalog.Variant{
		ID:           imageVariantID(asset.Path, result.Width, targetFormat),
		AssetPath:    asset.Path,
		ArtifactPath: targetPath,
		Size:         int64(len(result.Payload)),
		MediaType:    result.MediaType,
		SourceHash:   asset.SourceHash,
		ETag:         imageVariantETag(asset.SourceHash, result.Width, targetFormat),
		Format:       targetFormat,
		Width:        result.Width,
		Metadata: catalog.MetadataWithModTime(cxmapping.NewMapFrom(map[string]string{
			"stage":   "image",
			"backend": s.engine.Name(),
		}), time.Now()),
	}, nil
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
