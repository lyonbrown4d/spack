package source_test

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/source"
)

func TestNewSourceForTestSupportsLocalBackend(t *testing.T) {
	src, err := source.NewSourceForTest(&config.Assets{
		Backend: config.SourceBackendLocal,
		Root:    t.TempDir(),
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if src == nil {
		t.Fatal("expected local source instance")
	}
}

func TestNewSourceForTestRejectsUnsupportedBackend(t *testing.T) {
	_, err := source.NewSourceForTest(&config.Assets{
		Backend: "memory",
		Root:    t.TempDir(),
	}, slog.New(slog.DiscardHandler))
	if err == nil {
		t.Fatal("expected unsupported backend error")
	}
	if !strings.Contains(err.Error(), "unsupported assets backend") {
		t.Fatalf("expected unsupported backend error, got %v", err)
	}
	if !strings.Contains(err.Error(), "supported: local") {
		t.Fatalf("expected supported backend list in error, got %v", err)
	}
}
