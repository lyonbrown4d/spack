package config_test

import (
	"slices"
	"testing"

	"github.com/lyonbrown4d/spack/internal/config"
)

func TestNormalizeSourceBackendDefaultsToLocal(t *testing.T) {
	if got := config.NormalizeSourceBackend(""); got != config.SourceBackendLocal {
		t.Fatalf("expected default backend %q, got %q", config.SourceBackendLocal, got)
	}
}

func TestNormalizeSourceBackendNormalizesCaseAndWhitespace(t *testing.T) {
	if got := config.NormalizeSourceBackend(" LOCAL "); got != config.SourceBackendLocal {
		t.Fatalf("expected normalized backend %q, got %q", config.SourceBackendLocal, got)
	}
}

func TestSupportedSourceBackendNames(t *testing.T) {
	got := config.SupportedSourceBackendNames()
	if !slices.Equal(got.Values(), []string{"local"}) {
		t.Fatalf("unexpected supported source backends %#v", got.Values())
	}
}
