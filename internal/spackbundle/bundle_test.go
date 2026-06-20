package spackbundle_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

func TestWriteAndExtractBundle(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "index.html"), []byte("<html></html>"))
	writeTestFile(t, filepath.Join(root, "assets", "app.js"), []byte("console.log(1)"))

	output := filepath.Join(t.TempDir(), "app.spack")
	summary, err := spackbundle.Write(context.Background(), spackbundle.WriteOptions{
		Output: output,
		Root:   root,
		Now:    func() time.Time { return time.Unix(1, 0).UTC() },
		Files: []spackbundle.File{
			{Path: "index.html", FullPath: filepath.Join(root, "index.html"), Kind: "asset", MediaType: "text/html"},
			{Path: "assets/app.js", FullPath: filepath.Join(root, "assets", "app.js"), Kind: "asset", MediaType: "text/javascript"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Files != 2 {
		t.Fatalf("expected 2 files, got %d", summary.Files)
	}

	extracted, err := spackbundle.Extract(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := extracted.Cleanup(); err != nil {
			t.Fatal(err)
		}
	}()
	if extracted.Index.APIVersion != spackbundle.FormatVersion {
		t.Fatalf("unexpected index version %q", extracted.Index.APIVersion)
	}
	assertTestFile(t, filepath.Join(extracted.Root, "index.html"), []byte("<html></html>"))
	assertTestFile(t, filepath.Join(extracted.Root, "assets", "app.js"), []byte("console.log(1)"))
	if _, err := os.Stat(filepath.Join(extracted.Root, ".spack", "index.bin")); !os.IsNotExist(err) {
		t.Fatalf("expected metadata index to be skipped during extraction, got %v", err)
	}
}

func TestWriteRejectsReservedMetadataPath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".spack", "index.bin")
	writeTestFile(t, path, []byte("bad"))

	_, err := spackbundle.Write(context.Background(), spackbundle.WriteOptions{
		Output: filepath.Join(t.TempDir(), "app.spack"),
		Root:   root,
		Files:  []spackbundle.File{{Path: ".spack/index.bin", FullPath: path}},
	})
	if err == nil {
		t.Fatal("expected reserved metadata path to be rejected")
	}
}

func writeTestFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertTestFile(t *testing.T, path string, want []byte) {
	t.Helper()
	root, name := filepath.Split(path)
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := rootHandle.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}()
	file, err := rootHandle.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}()
	got, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("unexpected file content for %s: %q", path, got)
	}
}
