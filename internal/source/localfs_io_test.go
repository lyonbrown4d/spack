package source_test

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
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

func TestLocalFSCleanupClosesAnchoredRoot(t *testing.T) {
	root := t.TempDir()
	fullPath := filepath.Join(root, "app.js")
	writeLocalFSTestFile(t, fullPath, []byte("content"))
	src := newLocalFSForTest(t, root)

	if err := src.Cleanup(); err != nil {
		t.Fatal(err)
	}
	file, _, err := src.OpenFile(fullPath)
	if file != nil {
		if closeErr := file.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	if !errors.Is(err, fs.ErrClosed) {
		t.Fatalf("expected closed filesystem error, got %v", err)
	}
}

func TestLocalFSOpenFileConcurrent(t *testing.T) {
	root := t.TempDir()
	fullPath := filepath.Join(root, "app.js")
	writeLocalFSTestFile(t, fullPath, []byte("content"))
	src := newLocalFSForTest(t, root)

	const workers = 16
	const iterations = 20
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			<-start
			if err := repeatedlyReadLocalFSFile(src, fullPath, iterations); err != nil {
				errs <- err
			}
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func repeatedlyReadLocalFSFile(src *source.LocalFS, fullPath string, iterations int) error {
	for range iterations {
		file, _, err := src.OpenFile(fullPath)
		if err != nil {
			return fmt.Errorf("open local source file: %w", err)
		}
		body, readErr := io.ReadAll(file)
		closeErr := file.Close()
		if readErr != nil {
			return fmt.Errorf("read local source file: %w", readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close local source file: %w", closeErr)
		}
		if string(body) != "content" {
			return errors.New("unexpected concurrent file body")
		}
	}
	return nil
}
func TestLocalFSDirectSourceDoesNotExposeTrustedReadOnlyPath(t *testing.T) {
	root := t.TempDir()
	fullPath := filepath.Join(root, "app.js")
	writeLocalFSTestFile(t, fullPath, []byte("content"))
	src := newLocalFSForTest(t, root)

	rootFS, relativePath, trusted, err := src.TrustedReadOnlyPath(fullPath)
	if err != nil {
		t.Fatal(err)
	}
	if trusted || rootFS != nil || relativePath != "" {
		t.Fatal("expected direct source to withhold trusted read-only capability")
	}
}

func BenchmarkLocalFSOpenFile(b *testing.B) {
	root := b.TempDir()
	fullPath := filepath.Join(root, "app.js")
	if err := os.WriteFile(fullPath, []byte("content"), 0o600); err != nil {
		b.Fatal(err)
	}
	src, ok, err := source.NewLocalDirectory(root)
	if err != nil {
		b.Fatal(err)
	}
	if !ok {
		b.Fatal("expected local directory source")
	}
	b.Cleanup(func() {
		if cleanupErr := src.Cleanup(); cleanupErr != nil {
			b.Fatal(cleanupErr)
		}
	})
	b.ReportAllocs()

	for b.Loop() {
		file, _, openErr := src.OpenFile(fullPath)
		if openErr != nil {
			b.Fatal(openErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			b.Fatal(closeErr)
		}
	}
}
