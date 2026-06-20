package cmd_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lyonbrown4d/spack/cmd"
)

func TestCompileBundleRejectsBundleInput(t *testing.T) {
	_, err := cmd.CompileBundleForTest(context.Background(), filepath.Join(t.TempDir(), "app.spack"), filepath.Join(t.TempDir(), "out.spack"))
	if err == nil {
		t.Fatal("expected bundle input to be rejected")
	}
	if !strings.Contains(err.Error(), "compile input must be an asset directory") {
		t.Fatalf("expected directory input error, got %v", err)
	}
}
