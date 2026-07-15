//go:build spack_libvips

package pipeline_test

import (
	"context"
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/pipeline"
	"os"
	"path/filepath"
	"testing"
)

func TestImageStagePlanSchedulesWidthVariant(t *testing.T) {
	cat := catalog.NewInMemoryCatalog()
	asset := &catalog.Asset{
		Path:       "hero.jpg",
		FullPath:   "hero.jpg",
		MediaType:  "image/jpeg",
		SourceHash: "hash-1",
		ETag:       "\"hash-1\"",
	}
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}

	stage := pipeline.NewImageStageForTest(&config.Image{
		Enable: true,
		Widths: "640,1280",
	}, newTestStore(t.TempDir()), cat)

	tasks := stage.Plan(asset, pipeline.Request{
		AssetPath:       asset.Path,
		PreferredWidths: cxlist.NewList(640),
	})
	values := tasks.Values()
	if tasks.Len() != 1 || values[0].Width != 640 {
		t.Fatalf("unexpected image tasks: %#v", values)
	}
}

func TestImageStagePlanSchedulesFormatVariant(t *testing.T) {
	cat := catalog.NewInMemoryCatalog()
	asset := &catalog.Asset{
		Path:       "hero.png",
		FullPath:   "hero.png",
		MediaType:  "image/png",
		SourceHash: "hash-1",
		ETag:       "\"hash-1\"",
	}
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}

	stage := pipeline.NewImageStageForTest(&config.Image{
		Enable: true,
		Widths: "640,1280",
	}, newTestStore(t.TempDir()), cat)

	tasks := stage.Plan(asset, pipeline.Request{
		AssetPath:        asset.Path,
		PreferredFormats: cxlist.NewList("jpeg"),
	})
	if tasks.Len() != 1 {
		t.Fatalf("expected one format task, got %d", tasks.Len())
	}
	values := tasks.Values()
	if values[0].Width != 0 || values[0].Format != "jpeg" {
		t.Fatalf("unexpected format task: %#v", values[0])
	}
}

func TestImageStagePlanIgnoresUnsupportedFormatVariant(t *testing.T) {
	cat := catalog.NewInMemoryCatalog()
	asset := &catalog.Asset{
		Path:       "hero.png",
		FullPath:   "hero.png",
		MediaType:  "image/png",
		SourceHash: "hash-1",
		ETag:       "\"hash-1\"",
	}
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}

	stage := pipeline.NewImageStageForTest(&config.Image{
		Enable: true,
	}, newTestStore(t.TempDir()), cat)

	tasks := stage.Plan(asset, pipeline.Request{
		AssetPath:        asset.Path,
		PreferredFormats: cxlist.NewList("gif"),
	})
	if tasks.Len() != 0 {
		t.Fatalf("expected unsupported gif format to be ignored, got %#v", tasks.Values())
	}
}

func TestImageStageExecuteCreatesResizedVariant(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "hero.jpg")
	writeJPEGFixture(t, sourcePath, 1200, 800)

	cat := catalog.NewInMemoryCatalog()
	asset := &catalog.Asset{
		Path:       "hero.jpg",
		FullPath:   sourcePath,
		Size:       fileSize(t, sourcePath),
		MediaType:  "image/jpeg",
		SourceHash: "hash-1",
		ETag:       "\"hash-1\"",
	}
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}

	stage := pipeline.NewImageStageForTest(&config.Image{
		Enable:      true,
		JPEGQuality: 70,
	}, newTestStore(filepath.Join(dir, "cache")), cat)

	variant, err := stage.Execute(context.Background(), pipeline.Task{
		AssetPath: asset.Path,
		Width:     640,
	}, asset)
	if err != nil {
		t.Fatal(err)
	}
	if variant == nil {
		t.Fatal("expected image variant to be created")
		return
	}
	if variant.Width != 640 {
		t.Fatalf("expected width 640, got %d", variant.Width)
	}
	if _, err := os.Stat(variant.ArtifactPath); err != nil {
		t.Fatalf("expected artifact to exist: %v", err)
	}
}

func TestImageStageExecuteCreatesFormatVariant(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "hero.png")
	writePNGFixture(t, sourcePath, 1200, 800)

	cat := catalog.NewInMemoryCatalog()
	asset := &catalog.Asset{
		Path:       "hero.png",
		FullPath:   sourcePath,
		Size:       fileSize(t, sourcePath),
		MediaType:  "image/png",
		SourceHash: "hash-1",
		ETag:       "\"hash-1\"",
	}
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}

	stage := pipeline.NewImageStageForTest(&config.Image{
		Enable:      true,
		JPEGQuality: 70,
	}, newTestStore(filepath.Join(dir, "cache")), cat)

	variant, err := stage.Execute(context.Background(), pipeline.Task{
		AssetPath: asset.Path,
		Format:    "jpeg",
		Width:     0,
	}, asset)
	if err != nil {
		t.Fatal(err)
	}
	if variant == nil {
		t.Fatal("expected format variant to be created")
		return
	}
	if variant.Format != "jpeg" || variant.MediaType != "image/jpeg" {
		t.Fatalf("unexpected format variant: %#v", variant)
	}
	if _, err := os.Stat(variant.ArtifactPath); err != nil {
		t.Fatalf("expected artifact to exist: %v", err)
	}
}
