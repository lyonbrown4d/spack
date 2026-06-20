package cmd_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lyonbrown4d/spack/cmd"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

func TestInspectAssetsReportsBundleSummary(t *testing.T) {
	bundlePath, createdAt := newInspectBundleForTest(t)

	cfg := config.DefaultConfig()
	cfg.Assets.Root = bundlePath
	report, err := cmd.InspectAssetsForTest(context.Background(), &cfg)
	if err != nil {
		t.Fatal(err)
	}

	assertInspectBundleReport(t, report, createdAt)
}

func newInspectBundleForTest(t *testing.T) (string, time.Time) {
	t.Helper()

	root := t.TempDir()
	indexPath := filepath.Join(root, "index.html")
	sidecarPath := filepath.Join(root, "index.html.br")
	writeInspectTestFile(t, indexPath, []byte("<h1>bundle</h1>"))
	writeInspectTestFile(t, sidecarPath, []byte("compressed"))

	bundlePath := filepath.Join(t.TempDir(), "app.spack")
	createdAt := time.Unix(1_725_000_000, 0).UTC()
	if _, err := spackbundle.Write(context.Background(), spackbundle.WriteOptions{
		Output: bundlePath,
		Root:   root,
		Now: func() time.Time {
			return createdAt
		},
		Files: []spackbundle.File{
			{
				Path:       "index.html",
				FullPath:   indexPath,
				Kind:       "asset",
				MediaType:  "text/html",
				SourceHash: "hash-index",
				ETag:       `"hash-index"`,
			},
			{
				Path:       "index.html.br",
				FullPath:   sidecarPath,
				Kind:       "source_sidecar",
				MediaType:  "text/html",
				SourceHash: "hash-index",
				ETag:       `"hash-sidecar"`,
				AssetPath:  "index.html",
				Encoding:   "br",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return bundlePath, createdAt
}

func assertInspectBundleReport(t *testing.T, report cmd.InspectReportForTest, createdAt time.Time) {
	t.Helper()

	if report.Bundle == nil {
		t.Fatal("expected bundle summary")
	}
	if report.SourceType != "bundle" {
		t.Fatalf("expected source_type bundle, got %q", report.SourceType)
	}
	if report.AssetCount != 1 {
		t.Fatalf("expected 1 scanned asset, got %d", report.AssetCount)
	}
	if report.Bundle.FormatVersion != spackbundle.FormatVersion {
		t.Fatalf("expected bundle format version %q, got %q", spackbundle.FormatVersion, report.Bundle.FormatVersion)
	}
	if !report.Bundle.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected bundle created_at %s, got %s", createdAt, report.Bundle.CreatedAt)
	}
	if report.Bundle.FileCount != 2 {
		t.Fatalf("expected 2 bundle files, got %d", report.Bundle.FileCount)
	}
	if report.Bundle.AssetCount != 1 {
		t.Fatalf("expected 1 bundle asset, got %d", report.Bundle.AssetCount)
	}
	if report.Bundle.SourceSidecarCount != 1 {
		t.Fatalf("expected 1 bundle sidecar, got %d", report.Bundle.SourceSidecarCount)
	}
	if report.Bundle.CompressedFileCount != 1 {
		t.Fatalf("expected 1 compressed bundle file, got %d", report.Bundle.CompressedFileCount)
	}
}

func writeInspectTestFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}
