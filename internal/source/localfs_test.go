package source_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/requestpath"
	"github.com/lyonbrown4d/spack/internal/source"
)

func TestNewLocalFSRequiresRoot(t *testing.T) {
	_, err := source.NewLocalFS(&config.Assets{}, slog.New(slog.DiscardHandler))
	if err == nil {
		t.Fatal("expected error for empty root")
	}
}

func TestNewLocalFSRequiresDirectory(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "spack-file-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}()

	_, err = source.NewLocalFS(&config.Assets{Root: file.Name()}, slog.New(slog.DiscardHandler))
	if err == nil {
		t.Fatal("expected error for file root")
	}
}

func TestNewLocalFSRejectsSymlinkRoot(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "root-link")
	createSymlinkOrSkip(t, target, link)

	_, err := source.NewLocalFS(&config.Assets{Root: link}, slog.New(slog.DiscardHandler))
	if !errors.Is(err, source.ErrSymlinkNotAllowed) {
		t.Fatalf("expected ErrSymlinkNotAllowed, got %v", err)
	}
}

func TestLocalFSFindFileRejectsPathEscapes(t *testing.T) {
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
	for _, rawPath := range []string{
		"../secret.txt",
		"nested/../../secret.txt",
		"..\\secret.txt",
		filepath.ToSlash(outside),
		"//server/share/secret.txt",
		"\\\\server\\share\\secret.txt",
	} {
		t.Run(rawPath, func(t *testing.T) {
			_, found, err := src.FindFile(rawPath)
			if err != nil {
				t.Fatalf("expected no error for rejected escape path, got %v", err)
			}
			if found {
				t.Fatal("expected escaped path not to be found")
			}
		})
	}
}

func TestLocalFSFindFileRejectsDoubleDecodedBackslashEscape(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	cleaned := requestpath.Clean("%2e%2e%5csecret.txt")
	src := newLocalFSForTest(t, root)
	_, found, err := src.FindFile(cleaned.Value)
	if err != nil {
		t.Fatalf("expected no error for rejected decoded escape path, got %v", err)
	}
	if found {
		t.Fatal("expected decoded backslash escape not to be found")
	}
}

func TestLocalFSFindFileRejectsSymlinkFile(t *testing.T) {
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
	_, _, err := src.FindFile("link.txt")
	if !errors.Is(err, source.ErrSymlinkNotAllowed) {
		t.Fatalf("expected ErrSymlinkNotAllowed, got %v", err)
	}
}

func TestLocalFSFindFileRejectsSymlinkDirectory(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	outsideDir := filepath.Join(parent, "outside")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outsideDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	createSymlinkOrSkip(t, outsideDir, filepath.Join(root, "linked"))

	src := newLocalFSForTest(t, root)
	_, _, err := src.FindFile("linked/secret.txt")
	if !errors.Is(err, source.ErrSymlinkNotAllowed) {
		t.Fatalf("expected ErrSymlinkNotAllowed, got %v", err)
	}
}

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

func TestLocalFSFindFileDetectsReplacedRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows may reuse directory file IDs for immediate same-path replacement")
	}

	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	oldRoot := filepath.Join(parent, "root-old")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app.js"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := newLocalFSForTest(t, root)

	if err := os.Rename(root, oldRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app.js"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := src.FindFile("app.js")
	if !errors.Is(err, source.ErrRootReplaced) {
		t.Fatalf("expected ErrRootReplaced, got %v", err)
	}
}

func TestLocalFSFindFileRejectsSymlinkRootReplacement(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	oldRoot := filepath.Join(parent, "root-old")
	outsideRoot := filepath.Join(parent, "outside")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outsideRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	src := newLocalFSForTest(t, root)

	if err := os.Rename(root, oldRoot); err != nil {
		t.Fatal(err)
	}
	createSymlinkOrSkip(t, outsideRoot, root)

	_, _, err := src.FindFile("app.js")
	if !errors.Is(err, source.ErrSymlinkNotAllowed) {
		t.Fatalf("expected ErrSymlinkNotAllowed, got %v", err)
	}
}

func TestLocalFSWatchReportsFileChanges(t *testing.T) {
	root := t.TempDir()
	src, err := source.NewLocalFS(&config.Assets{Root: root}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	changes, err := src.Watch(ctx)
	if err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(root, "app.js")
	if err := os.WriteFile(target, []byte("console.log('watch');"), 0o600); err != nil {
		t.Fatal(err)
	}

	select {
	case change := <-changes:
		if change.Path != "app.js" {
			t.Fatalf("expected app.js watch event, got %#v", change)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for source watch event")
	}
}

func newLocalFSForTest(t *testing.T, root string) *source.LocalFS {
	t.Helper()

	src, err := source.NewLocalFS(&config.Assets{Root: root}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func createSymlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()

	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
}
