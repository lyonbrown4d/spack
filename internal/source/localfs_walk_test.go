package source_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

func TestLocalFSWalkRejectsSymlinkDirectory(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	createSymlinkOrSkip(t, realDir, filepath.Join(root, "linked"))

	src := newLocalFSForTest(t, root)
	err := src.Walk(func(source.File) error {
		return nil
	})
	if !errors.Is(err, source.ErrSymlinkNotAllowed) {
		t.Fatalf("expected ErrSymlinkNotAllowed, got %v", err)
	}
}

func TestLocalFSWalkRejectsSymlinkFile(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	createSymlinkOrSkip(t, outside, filepath.Join(root, "link.txt"))

	src := newLocalFSForTest(t, root)
	err := src.Walk(func(source.File) error {
		return nil
	})
	if !errors.Is(err, source.ErrSymlinkNotAllowed) {
		t.Fatalf("expected ErrSymlinkNotAllowed, got %v", err)
	}
}

func TestLocalFSWalkReturnsDeterministicSortedEntries(t *testing.T) {
	root := t.TempDir()
	writeLocalFSTestFile(t, filepath.Join(root, "z", "app.js"), []byte("console.log('z');"))
	writeLocalFSTestFile(t, filepath.Join(root, "a", "index.css"), []byte("body{}"))
	writeLocalFSTestFile(t, filepath.Join(root, "index.html"), []byte("<html></html>"))

	src := newLocalFSForTest(t, root)
	var paths []string
	if err := src.Walk(func(file source.File) error {
		paths = append(paths, file.Path)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	want := []string{
		".",
		"a",
		"a/index.css",
		"index.html",
		"z",
		"z/app.js",
	}
	if !slices.Equal(paths, want) {
		t.Fatalf("expected deterministic walk paths %#v, got %#v", want, paths)
	}
}

func TestLocalFSWalkBundleSourceUsesExtractedLocalPaths(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "assets", "app.js")
	writeLocalFSTestFile(t, sourcePath, []byte("console.log('bundle');"))

	bundlePath := filepath.Join(t.TempDir(), "app.spack")
	if _, err := spackbundle.Write(t.Context(), spackbundle.WriteOptions{
		Output: bundlePath,
		Root:   root,
		Files: []spackbundle.File{
			{Path: "assets/app.js", FullPath: sourcePath},
		},
	}); err != nil {
		t.Fatal(err)
	}

	src := newLocalFSForTest(t, bundlePath)
	var paths []string
	if err := src.Walk(func(file source.File) error {
		paths = append(paths, file.Path)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	want := []string{"assets/app.js"}
	if !slices.Equal(paths, want) {
		t.Fatalf("expected bundle walk paths %#v, got %#v", want, paths)
	}
}
