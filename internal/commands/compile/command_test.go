package compilecmd_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	compilecmd "github.com/lyonbrown4d/spack/internal/commands/compile"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

func TestCompileBundleRejectsBundleInput(t *testing.T) {
	_, err := compilecmd.BundleForTest(filepath.Join(t.TempDir(), "app.spack"), filepath.Join(t.TempDir(), "out.spack"))
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

	if _, err := compilecmd.BundleForTest(root, output); err != nil {
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

func TestCompileBundleWritesReadableOutput(t *testing.T) {
	root := t.TempDir()
	writeCompileTestFile(t, filepath.Join(root, "index.html"), []byte("<h1>ok</h1>"))
	output := filepath.Join(t.TempDir(), "app.spack")

	if _, err := compilecmd.BundleForTest(root, output); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	got := info.Mode().Perm()
	if got&0o444 != 0o444 {
		t.Fatalf("expected bundle to be readable, got mode %04o", got)
	}
	if runtime.GOOS != "windows" && got != 0o644 {
		t.Fatalf("expected bundle mode 0644, got %04o", got)
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
