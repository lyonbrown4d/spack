package assetprofile_test

import (
	"os"
	"path/filepath"
	"testing"

	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/assetprofile"
	"github.com/lyonbrown4d/spack/internal/catalog"
)

func TestAnalyzeAssetsReportsByteProfile(t *testing.T) {
	root := t.TempDir()
	textPath := filepath.Join(root, "app.js")
	writeProfileTestFile(t, textPath, []byte("aaaaaaaaaa\nbbbbbbbbbb\n"))

	assets := cxmapping.NewMapFrom(map[string]*catalog.Asset{
		"app.js": {
			Path:     "app.js",
			FullPath: textPath,
			Size:     22,
		},
	})

	summary := assetprofile.AnalyzeAssets(assets, assetprofile.Options{MaxSampleBytes: 1024, TopByteCount: 2})

	if summary.AssetCount != 1 {
		t.Fatalf("expected 1 asset, got %d", summary.AssetCount)
	}
	if summary.ProfiledAssetCount != 1 {
		t.Fatalf("expected 1 profiled asset, got %d", summary.ProfiledAssetCount)
	}
	if summary.SampledBytes != 22 {
		t.Fatalf("expected 22 sampled bytes, got %d", summary.SampledBytes)
	}
	if summary.EstimatedCompressibility != "high" {
		t.Fatalf("expected high compressibility, got %q", summary.EstimatedCompressibility)
	}
	if len(summary.TopBytes) == 0 || summary.TopBytes[0].Value != 'a' {
		t.Fatalf("expected top byte 'a', got %#v", summary.TopBytes)
	}
}

func TestAnalyzeAssetsMarksTruncatedSample(t *testing.T) {
	root := t.TempDir()
	textPath := filepath.Join(root, "app.js")
	writeProfileTestFile(t, textPath, []byte("0123456789"))

	assets := cxmapping.NewMapFrom(map[string]*catalog.Asset{
		"app.js": {
			Path:     "app.js",
			FullPath: textPath,
			Size:     10,
		},
	})

	summary := assetprofile.AnalyzeAssets(assets, assetprofile.Options{MaxSampleBytes: 4, TopByteCount: 2})

	if !summary.Truncated {
		t.Fatal("expected truncated sample")
	}
	if summary.SampledBytes != 4 {
		t.Fatalf("expected 4 sampled bytes, got %d", summary.SampledBytes)
	}
}

func writeProfileTestFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}
