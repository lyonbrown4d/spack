package source_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

func TestNewLocalFSExtractsBundleToLocalFileSource(t *testing.T) {
	sourcePath, bundlePath := newBundleLocalFSFixture(t)
	src := openBundleLocalFS(t, bundlePath)
	file := findBundleLocalFSFile(t, src)

	assertExtractedBundleLocalFSFile(t, file, sourcePath)
	assertBundleLocalFSCleanup(t, src, file)
}

func newBundleLocalFSFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	sourcePath := filepath.Join(root, "assets", "app.js")
	writeBundleLocalFSSource(t, sourcePath)
	return sourcePath, writeBundleLocalFSBundle(t, root, sourcePath)
}

func writeBundleLocalFSSource(t *testing.T, sourcePath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("console.log('bundle');"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeBundleLocalFSBundle(t *testing.T, root, sourcePath string) string {
	t.Helper()
	bundlePath := filepath.Join(t.TempDir(), "app.spack")
	_, err := spackbundle.Write(context.Background(), spackbundle.WriteOptions{
		Output: bundlePath,
		Root:   root,
		Files: []spackbundle.File{
			{
				Path:       "assets/app.js",
				FullPath:   sourcePath,
				Kind:       "asset",
				MediaType:  "application/javascript",
				SourceHash: "hash-app",
				ETag:       `"hash-app"`,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return bundlePath
}

func openBundleLocalFS(t *testing.T, bundlePath string) *source.LocalFS {
	t.Helper()
	src, err := source.NewLocalFS(&config.Assets{Root: bundlePath}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func findBundleLocalFSFile(t *testing.T, src *source.LocalFS) source.File {
	t.Helper()
	file, found, err := src.FindFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected bundle file to be found")
	}
	return file
}

func assertExtractedBundleLocalFSFile(t *testing.T, file source.File, sourcePath string) {
	t.Helper()
	if spackbundle.IsReference(file.FullPath) {
		t.Fatalf("expected extracted local path, got bundle reference %q", file.FullPath)
	}
	if file.FullPath == sourcePath {
		t.Fatalf("expected extracted path to differ from source path %q", sourcePath)
	}
	if file.Size != int64(len("console.log('bundle');")) {
		t.Fatalf("unexpected bundle size: %d", file.Size)
	}
	if file.MediaType != "application/javascript" {
		t.Fatalf("expected bundle media type metadata, got %q", file.MediaType)
	}
	if file.SourceHash != "hash-app" {
		t.Fatalf("expected bundle source hash metadata, got %q", file.SourceHash)
	}
	if file.ETag != `"hash-app"` {
		t.Fatalf("expected bundle etag metadata, got %q", file.ETag)
	}
	if _, err := os.Stat(file.FullPath); err != nil {
		t.Fatal(err)
	}
}

func assertBundleLocalFSCleanup(t *testing.T, src *source.LocalFS, file source.File) {
	t.Helper()
	extractedRoot := filepath.Dir(filepath.Dir(file.FullPath))
	if err := src.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(extractedRoot); !os.IsNotExist(err) {
		t.Fatalf("expected extracted root to be cleaned up, got %v", err)
	}
}
