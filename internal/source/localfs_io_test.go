package source_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lyonbrown4d/spack/internal/source"
)

func TestLocalFSReadFile(t *testing.T) {
	root := t.TempDir()
	fullPath := filepath.Join(root, "nested", "app.js")
	writeLocalFSTestFile(t, fullPath, []byte("console.log('safe');"))

	src := newLocalFSForTest(t, root)
	body, err := src.ReadFile(fullPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "console.log('safe');" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestLocalFSReadFileRejectsOutsideRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	src := newLocalFSForTest(t, root)
	if _, err := src.ReadFile(outside); err == nil {
		t.Fatal("expected outside-root read to fail")
	}
}

func TestLocalFSReadFileRejectsSymlink(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.txt")
	createSymlinkOrSkip(t, outside, link)

	src := newLocalFSForTest(t, root)
	_, err := src.ReadFile(link)
	if !errors.Is(err, source.ErrSymlinkNotAllowed) {
		t.Fatalf("expected ErrSymlinkNotAllowed, got %v", err)
	}
}

func TestLocalFSOpenFileDetectsReplacedRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows may reuse directory file IDs for immediate same-path replacement")
	}

	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	oldRoot := filepath.Join(parent, "root-old")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	fullPath := filepath.Join(root, "app.js")
	if err := os.WriteFile(fullPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := newLocalFSForTest(t, root)

	if err := os.Rename(root, oldRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}

	file, _, err := src.OpenFile(fullPath)
	if file != nil {
		if closeErr := file.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	if !errors.Is(err, source.ErrRootReplaced) {
		t.Fatalf("expected ErrRootReplaced, got %v", err)
	}
}
