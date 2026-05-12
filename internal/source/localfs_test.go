package source_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/source"
)

func TestNewLocalFSRequiresRoot(t *testing.T) {
	_, err := source.NewLocalFSForTest(&config.Assets{}, slog.New(slog.DiscardHandler))
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

	_, err = source.NewLocalFSForTest(&config.Assets{Root: file.Name()}, slog.New(slog.DiscardHandler))
	if err == nil {
		t.Fatal("expected error for file root")
	}
}

func TestLocalFSWatchReportsFileChanges(t *testing.T) {
	root := t.TempDir()
	src, err := source.NewLocalFSForTest(&config.Assets{Root: root}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	watcher, ok := src.(source.Watcher)
	if !ok {
		t.Fatal("expected local filesystem source to support watching")
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	changes, err := watcher.Watch(ctx)
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
