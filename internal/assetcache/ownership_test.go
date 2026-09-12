package assetcache_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/source"
)

func TestCacheStopPreservesBorrowedFileSource(t *testing.T) {
	files := newOwnershipTestSource(t)
	t.Cleanup(func() {
		if err := files.Cleanup(); err != nil {
			t.Errorf("cleanup borrowed test source: %v", err)
		}
	})

	cache := assetcache.NewCacheWithSourceForTest(
		config.MemoryCache{},
		slog.New(slog.DiscardHandler),
		files,
	)
	if cache == nil {
		t.Fatal("expected cache construction to succeed")
	}
	_, owned := assetcache.FileSourceForTest(cache)
	if owned {
		t.Fatal("expected injected file source to be borrowed")
	}

	if err := assetcache.StopForTest(cache); err != nil {
		t.Fatal(err)
	}
	assertOwnershipTestSourceReadable(t, files)
}

func TestCacheStopCleansOwnedFallbackFileSource(t *testing.T) {
	root := newOwnershipTestRoot(t)
	cache := assetcache.NewCacheWithRootForTest(
		config.MemoryCache{},
		slog.New(slog.DiscardHandler),
		root,
	)
	if cache == nil {
		t.Fatal("expected cache construction to succeed")
	}
	files, owned := assetcache.FileSourceForTest(cache)
	if !owned {
		t.Fatal("expected fallback file source to be owned")
	}

	if err := assetcache.StopForTest(cache); err != nil {
		t.Fatal(err)
	}
	assertOwnershipTestSourceClosed(t, files)
	if err := assetcache.StopForTest(cache); err != nil {
		t.Fatalf("expected repeated stop to succeed: %v", err)
	}
}

func newOwnershipTestRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "asset.txt"), []byte("asset"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func newOwnershipTestSource(t *testing.T) *source.LocalFS {
	t.Helper()

	files, ok, err := source.NewLocalDirectory(newOwnershipTestRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected local directory source")
	}
	return files
}

func assertOwnershipTestSourceReadable(t *testing.T, files *source.LocalFS) {
	t.Helper()

	_, found, err := files.FindFile("asset.txt")
	if err != nil {
		t.Fatalf("expected file source to remain readable: %v", err)
	}
	if !found {
		t.Fatal("expected test asset to remain available")
	}
}

func assertOwnershipTestSourceClosed(t *testing.T, files *source.LocalFS) {
	t.Helper()

	if _, _, err := files.FindFile("asset.txt"); err == nil {
		t.Fatal("expected file source to be closed")
	}
}
