package server

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/source"
)

func TestMergeServerFileSourcesCleansDiscardedOwnedDuplicate(t *testing.T) {
	root := t.TempDir()
	writeServerFileSourceFixture(t, root)

	borrowed := newServerLocalDirectory(t, root)
	duplicate := newServerLocalDirectory(t, root)
	t.Cleanup(func() {
		if err := borrowed.Cleanup(); err != nil {
			t.Fatal(err)
		}
	})

	merged := mergeServerFileSources(
		newServerFileSourcesWithEntries(serverFileSource{files: borrowed}),
		newServerFileSourcesWithEntries(serverFileSource{files: duplicate, owned: true}),
	)
	if merged == nil || merged.sourceCount() != 1 {
		t.Fatalf("expected one merged source, got %#v", merged)
	}
	if _, _, err := duplicate.OpenFile(filepath.Join(root, "app.js")); !errors.Is(err, fs.ErrClosed) {
		t.Fatalf("expected discarded owned duplicate to be closed, got %v", err)
	}
	file, _, err := borrowed.OpenFile(filepath.Join(root, "app.js"))
	if err != nil {
		t.Fatalf("expected retained borrowed source to remain open: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestServerFileSourcesCleanupIsIdempotentAndPreservesBorrowed(t *testing.T) {
	borrowedRoot := t.TempDir()
	ownedRoot := filepath.Join(t.TempDir(), "owned")
	if err := os.Mkdir(ownedRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeServerFileSourceFixture(t, borrowedRoot)
	writeServerFileSourceFixture(t, ownedRoot)

	borrowed := newServerLocalDirectory(t, borrowedRoot)
	owned := newServerLocalDirectory(t, ownedRoot)
	t.Cleanup(func() {
		if err := borrowed.Cleanup(); err != nil {
			t.Fatal(err)
		}
	})

	sources := newServerFileSourcesWithEntries(
		serverFileSource{files: borrowed},
		serverFileSource{files: owned, owned: true},
	)
	if err := sources.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := sources.Cleanup(); err != nil {
		t.Fatalf("second cleanup must be idempotent: %v", err)
	}
	file, _, err := borrowed.OpenFile(filepath.Join(borrowedRoot, "app.js"))
	if err != nil {
		t.Fatalf("borrowed source was closed: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := owned.OpenFile(filepath.Join(ownedRoot, "app.js")); !errors.Is(err, fs.ErrClosed) {
		t.Fatalf("expected owned source to be closed, got %v", err)
	}
	if err := os.RemoveAll(ownedRoot); err != nil {
		t.Fatalf("remove owned source root after cleanup: %v", err)
	}
}

func TestPreparedServiceRebuildReusesOwnedFileSources(t *testing.T) {
	root := t.TempDir()
	writeServerFileSourceFixture(t, root)
	cfg := config.DefaultConfigForTest()
	cfg.Assets.Root = root
	cfg.Compression.CacheDir = filepath.Join(t.TempDir(), "missing-cache")
	svc := newPreparedService(
		&cfg,
		catalog.NewInMemoryCatalog(),
		slog.New(slog.DiscardHandler),
		nil,
		nil,
		nil,
	)
	t.Cleanup(func() {
		if err := svc.stop(context.Background()); err != nil {
			t.Fatal(err)
		}
	})

	if svc.fileSources == nil || svc.fileSources.empty() {
		t.Fatal("expected owned file source")
	}
	original := svc.fileSources.first()
	for range 3 {
		if err := svc.Rebuild(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := svc.fileSources.first(); got != original {
		t.Fatal("rebuild replaced an existing owned file source")
	}
}

func TestPreparedServiceStopReleasesOwnedSourcesAndPreservesBorrowed(t *testing.T) {
	parent := t.TempDir()
	ownedRoot := filepath.Join(parent, "owned")
	if err := os.Mkdir(ownedRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeServerFileSourceFixture(t, ownedRoot)

	cfg := config.DefaultConfigForTest()
	cfg.Assets.Root = ownedRoot
	cfg.Compression.CacheDir = filepath.Join(parent, "missing-cache")
	ownedService := newPreparedService(
		&cfg,
		catalog.NewInMemoryCatalog(),
		slog.New(slog.DiscardHandler),
		nil,
		nil,
		nil,
	)
	if err := ownedService.stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(ownedRoot); err != nil {
		t.Fatalf("remove root after prepared service stop: %v", err)
	}

	borrowedRoot := filepath.Join(parent, "borrowed")
	if err := os.Mkdir(borrowedRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeServerFileSourceFixture(t, borrowedRoot)
	borrowed := newServerLocalDirectory(t, borrowedRoot)
	t.Cleanup(func() {
		if err := borrowed.Cleanup(); err != nil {
			t.Fatal(err)
		}
	})
	cfg.Assets.Root = borrowedRoot
	borrowedService := newPreparedService(
		&cfg,
		catalog.NewInMemoryCatalog(),
		slog.New(slog.DiscardHandler),
		nil,
		nil,
		borrowed,
	)
	if err := borrowedService.stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	file, _, err := borrowed.OpenFile(filepath.Join(borrowedRoot, "app.js"))
	if err != nil {
		t.Fatalf("prepared service closed borrowed source: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestServerFileSourcesRefreshConcurrentWithRead(t *testing.T) {
	readRoot := t.TempDir()
	writeServerFileSourceFixture(t, readRoot)
	borrowed := newServerLocalDirectory(t, readRoot)
	t.Cleanup(func() {
		if err := borrowed.Cleanup(); err != nil {
			t.Fatal(err)
		}
	})

	sources := newServerFileSourcesFromSource(borrowed)
	t.Cleanup(func() {
		if err := sources.Cleanup(); err != nil {
			t.Fatal(err)
		}
	})

	const refreshCount = 32
	configs := newServerFileSourceRefreshConfigs(t, refreshCount)

	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Go(func() {
		<-start
		results <- refreshServerFileSourcesForTest(sources, configs)
	})
	workers.Go(func() {
		<-start
		results <- readServerFileSourcesForTest(sources, filepath.Join(readRoot, "app.js"), refreshCount*8)
	})
	close(start)
	workers.Wait()

	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if got := sources.sourceCount(); got != refreshCount+1 {
		t.Fatalf("expected %d sources after refresh, got %d", refreshCount+1, got)
	}
}

func newServerFileSourceRefreshConfigs(t *testing.T, count int) []config.Config {
	t.Helper()

	configs := make([]config.Config, count)
	for index := range count {
		root := filepath.Join(t.TempDir(), "assets")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		writeServerFileSourceFixture(t, root)
		cfg := config.DefaultConfigForTest()
		cfg.Assets.Root = root
		cfg.Compression.CacheDir = ""
		configs[index] = cfg
	}
	return configs
}
func refreshServerFileSourcesForTest(sources *serverFileSources, configs []config.Config) error {
	for index := range configs {
		if refreshed := refreshServerFileSources(sources, &configs[index], nil, nil); refreshed != sources {
			return errors.New("refresh replaced server file sources")
		}
	}
	return nil
}

func readServerFileSourcesForTest(sources *serverFileSources, path string, count int) error {
	for range count {
		body, err := sources.ReadFile(path)
		if err != nil {
			return err
		}
		if string(body) != "fixture" {
			return errors.New("read unexpected fixture body")
		}
	}
	return nil
}
func newServerLocalDirectory(t *testing.T, root string) *source.LocalFS {
	t.Helper()
	files, ok, err := source.NewLocalDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected local directory source")
	}
	return files
}

func writeServerFileSourceFixture(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "app.js"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
}
