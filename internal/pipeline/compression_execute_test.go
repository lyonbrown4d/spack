package pipeline_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/pipeline"
)

func TestCompressionStageExecuteCreatesVariant(t *testing.T) {
	assertCompressionVariantCreated(t, compressionVariantSpec{
		sourceHash: "hash-1",
		encoding:   "gzip",
		sourceBody: []byte(`{"message":"` + strings.Repeat("compressible-payload-", 256) + `"}`),
		suffix:     ".gz",
		applyLevel: func(cfg *config.Compression) {
			cfg.GzipLevel = 5
		},
	})
}

func TestCompressionStageExecuteCreatesZstdVariant(t *testing.T) {
	assertCompressionVariantCreated(t, compressionVariantSpec{
		sourceHash: "hash-zstd",
		encoding:   "zstd",
		sourceBody: []byte(`{"message":"` + strings.Repeat("zstd-payload-", 512) + `"}`),
		suffix:     ".zst",
		applyLevel: func(cfg *config.Compression) {
			cfg.ZstdLevel = 3
		},
	})
}

type compressionVariantSpec struct {
	sourceHash string
	encoding   string
	sourceBody []byte
	suffix     string
	applyLevel func(*config.Compression)
}

func assertCompressionVariantCreated(t *testing.T, spec compressionVariantSpec) {
	t.Helper()

	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "payload.json")
	if err := os.WriteFile(sourcePath, spec.sourceBody, 0o600); err != nil {
		t.Fatal(err)
	}

	cat := catalog.NewInMemoryCatalog()
	asset := &catalog.Asset{
		Path:       "payload.json",
		FullPath:   sourcePath,
		MediaType:  "application/json",
		SourceHash: spec.sourceHash,
		ETag:       "\"" + spec.sourceHash + "\"",
	}
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(dir, "cache")
	store := newTestStore(cacheDir)
	compression := &config.Compression{
		Enable:   true,
		Mode:     config.CompressionModeLazy,
		CacheDir: cacheDir,
		MinSize:  1,
	}
	if spec.applyLevel != nil {
		spec.applyLevel(compression)
	}

	stage := pipeline.NewCompressionStageForTest(compression, store, cat)
	variant, err := stage.Execute(t.Context(), pipeline.Task{
		AssetPath: asset.Path,
		Encoding:  spec.encoding,
	}, asset)
	if err != nil {
		t.Fatal(err)
	}
	if variant == nil {
		t.Fatal("expected variant to be created")
		return
	}
	if variant.Encoding != spec.encoding {
		t.Fatalf("expected %s encoding, got %q", spec.encoding, variant.Encoding)
	}

	expectedPath, err := store.PathFor(asset.Path, asset.SourceHash, "encoding", spec.suffix)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected artifact to exist: %v", err)
	}
}
