package pipeline

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
)

type recordingImageEngine struct {
	batches int
	results *cxlist.List[imageGenerateResult]
}

func (e *recordingImageEngine) Name() string {
	return "recording"
}

func (e *recordingImageEngine) SupportsSourceMediaType(string) bool {
	return true
}

func (e *recordingImageEngine) SupportedTargetFormats() *cxlist.List[string] {
	return cxlist.NewList("jpeg", "png")
}

func (e *recordingImageEngine) Generate(request imageGenerateRequest) (imageGenerateResult, error) {
	results, err := e.GenerateBatch(imageGenerateBatchRequest{
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

func (e *recordingImageEngine) GenerateBatch(imageGenerateBatchRequest) (*cxlist.List[imageGenerateResult], error) {
	e.batches++
	return e.results, nil
}

func TestImageStageExecuteBatchWritesMultipleVariantsWithOneEngineBatch(t *testing.T) {
	root := t.TempDir()
	asset := imageBatchAssetForTest(t, root, 10_000)
	cat := catalog.NewInMemoryCatalog()
	upsertImageBatchAssetForTest(t, cat, asset)

	engine := &recordingImageEngine{
		results: cxlist.NewList(
			imageBatchResultForTest(640, "jpeg", []byte("jpeg")),
			imageBatchResultForTest(320, "jpeg", []byte("small-jpeg")),
		),
	}
	stage := newImageStage(&config.Image{
		Enable:         true,
		Engine:         "builtin",
		JPEGQuality:    70,
		MinSavingRatio: 0.01,
	}, engine, batchTestStore{root: filepath.Join(root, "cache")}, cat)

	variants, err := stage.ExecuteBatch(Task{
		AssetPath: asset.Path,
		ImageVariants: cxlist.NewList(
			ImageVariantTask{Format: "jpeg", Width: 640},
			ImageVariantTask{Format: "jpeg", Width: 320},
		),
	}, asset)
	if err != nil {
		t.Fatal(err)
	}
	if engine.batches != 1 {
		t.Fatalf("expected one engine batch, got %d", engine.batches)
	}
	if variants.Len() != 2 {
		t.Fatalf("expected two variants, got %d", variants.Len())
	}
}

func TestImageStagePlanLimitsOutputVariants(t *testing.T) {
	asset := &catalog.Asset{
		Path:       "hero.jpg",
		FullPath:   "hero.jpg",
		Size:       10_000,
		MediaType:  "image/jpeg",
		SourceHash: "hash-1",
	}
	cat := catalog.NewInMemoryCatalog()
	upsertImageBatchAssetForTest(t, cat, asset)

	stage := newImageStage(&config.Image{
		Enable:            true,
		Engine:            "builtin",
		MaxOutputVariants: 1,
	}, &recordingImageEngine{}, batchTestStore{root: t.TempDir()}, cat)

	tasks := stage.Plan(asset, Request{
		AssetPath:        asset.Path,
		PreferredFormats: cxlist.NewList("jpeg"),
		PreferredWidths:  cxlist.NewList(640, 320),
	})
	task, ok := tasks.Get(0)
	if !ok || task.ImageVariants.Len() != 1 {
		t.Fatalf("expected one limited image variant task, got %#v", tasks.Values())
	}
}

func TestImageStageExecuteBatchSkipsLowBenefitVariants(t *testing.T) {
	root := t.TempDir()
	asset := imageBatchAssetForTest(t, root, 100)
	cat := catalog.NewInMemoryCatalog()
	upsertImageBatchAssetForTest(t, cat, asset)

	stage := newImageStage(&config.Image{
		Enable:         true,
		Engine:         "builtin",
		MinSavingRatio: 0.10,
		MinSavingBytes: 10,
		JPEGQuality:    70,
	}, &recordingImageEngine{
		results: cxlist.NewList(lowBenefitImageBatchResultForTest()),
	}, batchTestStore{root: filepath.Join(root, "cache")}, cat)

	_, err := stage.ExecuteBatch(Task{
		AssetPath:     asset.Path,
		ImageVariants: cxlist.NewList(ImageVariantTask{Format: "jpeg", Width: 640}),
	}, asset)
	if !IsVariantSkipped(err) {
		t.Fatalf("expected low-benefit variant to be skipped, got %v", err)
	}
}

func TestBuiltinImageEngineRejectsSourceByteLimit(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeInternalTempBytes(t, root, make([]byte, 128))

	_, err := builtinImageEngine{}.GenerateBatch(imageGenerateBatchRequest{
		SourcePath: sourcePath,
		Variants: cxlist.NewList(imageVariantGenerateRequest{
			TargetFormat: "png",
			TargetWidth:  16,
		}),
		Limits: imageGenerateLimits{MaxSourceBytes: 64},
	})
	if !IsVariantSkipped(err) {
		t.Fatalf("expected byte-limited source to be skipped, got %v", err)
	}
}

func TestBuiltinImageEngineRejectsSourcePixelLimit(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeInternalPNGFixture(t, root, 4, 4)

	_, err := builtinImageEngine{}.GenerateBatch(imageGenerateBatchRequest{
		SourcePath: sourcePath,
		Variants: cxlist.NewList(imageVariantGenerateRequest{
			TargetFormat: "png",
			TargetWidth:  2,
		}),
		Limits: imageGenerateLimits{MaxSourcePixels: 8},
	})
	if !IsVariantSkipped(err) {
		t.Fatalf("expected pixel-limited source to be skipped, got %v", err)
	}
}

func imageBatchAssetForTest(t *testing.T, root string, size int64) *catalog.Asset {
	t.Helper()

	sourcePath := filepath.Join(root, "hero.jpg")
	if err := os.WriteFile(sourcePath, make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
	return &catalog.Asset{
		Path:       "hero.jpg",
		FullPath:   sourcePath,
		Size:       size,
		MediaType:  "image/jpeg",
		SourceHash: "hash-1",
		ETag:       "\"hash-1\"",
	}
}

func imageBatchResultForTest(width int, format string, payload []byte) imageGenerateResult {
	return imageGenerateResult{
		Payload:      payload,
		Width:        width,
		Height:       width / 2,
		SourceWidth:  1280,
		SourceHeight: 640,
		SourceBytes:  10_000,
		TargetFormat: format,
		MediaType:    "image/jpeg",
		Extension:    ".jpg",
	}
}

func lowBenefitImageBatchResultForTest() imageGenerateResult {
	result := imageBatchResultForTest(640, "jpeg", make([]byte, 95))
	result.SourceBytes = 100
	return result
}

func upsertImageBatchAssetForTest(t *testing.T, cat catalog.Catalog, asset *catalog.Asset) {
	t.Helper()
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}
}

func writeInternalPNGFixture(t *testing.T, root string, width, height int) string {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: 255})
		}
	}
	file, err := os.CreateTemp(root, "image-*.png")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}()
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
	return file.Name()
}

func writeInternalTempBytes(t *testing.T, root string, body []byte) string {
	t.Helper()

	file, err := os.CreateTemp(root, "source-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}()
	if _, err := file.Write(body); err != nil {
		t.Fatal(err)
	}
	return file.Name()
}

type batchTestStore struct {
	root string
}

func (s batchTestStore) Root() string {
	return s.root
}

func (s batchTestStore) PathFor(assetPath, sourceHash, namespace, suffix string) string {
	return filepath.Join(s.root, namespace, sourceHash, filepath.Clean(assetPath)+suffix)
}

func (s batchTestStore) Write(path string, data []byte) error {
	_ = path
	_ = data
	return nil
}
