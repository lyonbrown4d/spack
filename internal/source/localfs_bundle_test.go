package source_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

func TestNewLocalFSBundleStatsRecordExtraction(t *testing.T) {
	fixture := newLocalFSBundleFixture(t)
	src, openErr := source.NewLocalFS(&config.Assets{Root: fixture.bundlePath}, slog.New(slog.DiscardHandler))
	if openErr != nil {
		t.Fatal(openErr)
	}
	t.Cleanup(func() {
		cleanupErr := src.Cleanup()
		if cleanupErr != nil {
			t.Fatal(cleanupErr)
		}
	})

	assertLocalFSBundleStats(t, src.Stats(), fixture.totalBytes)
	assertLocalFSBundleServesFromExtractedRoot(t, src, fixture.originalAppPath)
}

type localFSBundleFixture struct {
	bundlePath      string
	originalAppPath string
	totalBytes      int64
}

func newLocalFSBundleFixture(t *testing.T) localFSBundleFixture {
	t.Helper()
	assetRoot := t.TempDir()
	indexBody := []byte("<h1>bundle</h1>")
	appBody := []byte("console.log('bundle');")
	indexPath := filepath.Join(assetRoot, "index.html")
	appPath := filepath.Join(assetRoot, "assets", "app.js")
	writeLocalFSTestFile(t, indexPath, indexBody)
	writeLocalFSTestFile(t, appPath, appBody)

	bundlePath := filepath.Join(t.TempDir(), "app.spack")
	_, writeErr := spackbundle.Write(context.Background(), spackbundle.WriteOptions{
		Output: bundlePath,
		Root:   assetRoot,
		Files: []spackbundle.File{
			{Path: "index.html", FullPath: indexPath, Kind: "asset", MediaType: "text/html"},
			{Path: "assets/app.js", FullPath: appPath, Kind: "asset", MediaType: "text/javascript"},
		},
	})
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	return localFSBundleFixture{
		bundlePath:      bundlePath,
		originalAppPath: appPath,
		totalBytes:      int64(len(indexBody) + len(appBody)),
	}
}

func assertLocalFSBundleStats(t *testing.T, stats source.Stats, wantBytes int64) {
	t.Helper()
	if stats.Mode != source.SourceModeAOT {
		t.Fatalf("expected aot source mode, got %q", stats.Mode)
	}
	if stats.BundleExtractionFiles != 2 {
		t.Fatalf("expected 2 bundle files, got %d", stats.BundleExtractionFiles)
	}
	if stats.BundleExtractionBytes != wantBytes {
		t.Fatalf("expected %d extracted bytes, got %d", wantBytes, stats.BundleExtractionBytes)
	}
	if stats.BundleExtractionDuration <= 0 {
		t.Fatalf("expected extraction duration to be recorded, got %s", stats.BundleExtractionDuration)
	}
}

func assertLocalFSBundleServesFromExtractedRoot(t *testing.T, src *source.LocalFS, originalPath string) {
	t.Helper()
	file, found, findErr := src.FindFile("assets/app.js")
	if findErr != nil {
		t.Fatal(findErr)
	}
	if !found {
		t.Fatal("expected bundled asset to be found after startup extraction")
	}
	if file.FullPath == originalPath {
		t.Fatal("expected bundled asset to be served from extracted runtime directory")
	}
}
