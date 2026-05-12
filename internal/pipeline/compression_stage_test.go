package pipeline_test

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/pipeline"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCompressionStagePlanSkipsExistingVariant(t *testing.T) {
	cat := catalog.NewInMemoryCatalog()
	asset := &catalog.Asset{
		Path:       "app.js",
		FullPath:   "app.js",
		MediaType:  "application/javascript",
		SourceHash: "hash-1",
		ETag:       "\"hash-1\"",
	}
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}

	variantPath := filepath.Join(t.TempDir(), "app.js.br")
	if err := os.WriteFile(variantPath, []byte("compressed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cat.UpsertVariant(&catalog.Variant{
		ID:           "app.js.br",
		AssetPath:    asset.Path,
		ArtifactPath: variantPath,
		MediaType:    asset.MediaType,
		SourceHash:   asset.SourceHash,
		Encoding:     "br",
		ETag:         "\"hash-1-br\"",
	}); err != nil {
		t.Fatal(err)
	}

	stage := pipeline.NewCompressionStageForTest(&config.Compression{
		Enable: true,
		Mode:   config.CompressionModeLazy,
	}, newTestStore(t.TempDir()), cat)

	tasks := stage.Plan(asset, pipeline.Request{
		AssetPath:          asset.Path,
		PreferredEncodings: cxlist.NewList("br", "gzip", "zstd"),
	})
	if tasks.Len() != 2 {
		t.Fatalf("expected two tasks, got %d", tasks.Len())
	}
	values := tasks.Values()
	got := []string{values[0].Encoding, values[1].Encoding}
	if !slices.Equal(got, []string{"gzip", "zstd"}) {
		t.Fatalf("expected gzip and zstd tasks, got %#v", got)
	}
}

func TestCompressionStagePlanUsesConfiguredEncodingOrder(t *testing.T) {
	cat := catalog.NewInMemoryCatalog()
	asset := &catalog.Asset{
		Path:       "app.js",
		FullPath:   "app.js",
		MediaType:  "application/javascript",
		SourceHash: "hash-1",
		ETag:       "\"hash-1\"",
	}
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}

	stage := pipeline.NewCompressionStageForTest(&config.Compression{
		Enable:    true,
		Mode:      config.CompressionModeLazy,
		Encodings: "gzip,zstd",
	}, newTestStore(t.TempDir()), cat)

	tasks := stage.Plan(asset, pipeline.Request{AssetPath: asset.Path})
	if tasks.Len() != 2 {
		t.Fatalf("expected two tasks, got %d", tasks.Len())
	}
	values := tasks.Values()
	got := []string{values[0].Encoding, values[1].Encoding}
	if !slices.Equal(got, []string{"gzip", "zstd"}) {
		t.Fatalf("expected configured encoding order [gzip zstd], got %#v", got)
	}
}

func TestCompressionStagePlanFiltersDisabledRequestEncodings(t *testing.T) {
	cat := catalog.NewInMemoryCatalog()
	asset := &catalog.Asset{
		Path:       "app.js",
		FullPath:   "app.js",
		MediaType:  "application/javascript",
		SourceHash: "hash-1",
		ETag:       "\"hash-1\"",
	}
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}

	stage := pipeline.NewCompressionStageForTest(&config.Compression{
		Enable:    true,
		Mode:      config.CompressionModeLazy,
		Encodings: "br,gzip",
	}, newTestStore(t.TempDir()), cat)

	tasks := stage.Plan(asset, pipeline.Request{
		AssetPath:          asset.Path,
		PreferredEncodings: cxlist.NewList("zstd", "gzip"),
	})
	values := tasks.Values()
	if tasks.Len() != 1 || values[0].Encoding != "gzip" {
		t.Fatalf("expected only enabled gzip task, got %#v", values)
	}
}
