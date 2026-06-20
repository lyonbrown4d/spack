package cmd_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lyonbrown4d/spack/cmd"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
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

func TestCompileBundleExcludesOutputInsideAssetsRoot(t *testing.T) {
	root := t.TempDir()
	writeCompileTestFile(t, filepath.Join(root, "index.html"), []byte("<h1>ok</h1>"))
	output := filepath.Join(root, "app.spack")
	writeCompileTestFile(t, output, []byte("old bundle"))

	if _, err := cmd.CompileBundleForTest(context.Background(), root, output); err != nil {
		t.Fatal(err)
	}

	index, err := spackbundle.ReadIndex(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range index.Files {
		if file.Path == "app.spack" {
			t.Fatal("expected compiler output file to be excluded from bundle")
		}
	}
}

func writeCompileTestFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}
