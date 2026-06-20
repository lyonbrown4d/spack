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

func TestNewLocalFSServesBundleReference(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "assets", "app.js")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("console.log('bundle');"), 0o600); err != nil {
		t.Fatal(err)
	}
	bundlePath := filepath.Join(t.TempDir(), "app.spack")
	if _, err := spackbundle.Write(context.Background(), spackbundle.WriteOptions{
		Output: bundlePath,
		Root:   root,
		Files: []spackbundle.File{
			{Path: "assets/app.js", FullPath: sourcePath},
		},
	}); err != nil {
		t.Fatal(err)
	}

	src, err := source.NewLocalFS(&config.Assets{Root: bundlePath}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cleanupErr := src.Cleanup(); cleanupErr != nil {
			t.Fatal(cleanupErr)
		}
	})

	file, found, err := src.FindFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected bundle file to be found")
	}
	if file.Size != int64(len("console.log('bundle');")) {
		t.Fatalf("unexpected bundle size: %d", file.Size)
	}
}
