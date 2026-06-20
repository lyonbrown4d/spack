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

func TestValidateConfiguredAssetsRootAcceptsDirectoryAndBundle(t *testing.T) {
	root := t.TempDir()
	if err := cmd.ValidateConfiguredAssetsRootForTest(root); err != nil {
		t.Fatal(err)
	}

	assetPath := filepath.Join(root, "index.html")
	writeInspectTestFile(t, assetPath, []byte("<h1>ok</h1>"))
	bundlePath := filepath.Join(t.TempDir(), "app.spack")
	if _, err := spackbundle.Write(context.Background(), spackbundle.WriteOptions{
		Output: bundlePath,
		Root:   root,
		Files: []spackbundle.File{
			{Path: "index.html", FullPath: assetPath, Kind: "asset", MediaType: "text/html"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := cmd.ValidateConfiguredAssetsRootForTest(bundlePath); err != nil {
		t.Fatal(err)
	}
}

func TestValidateConfiguredAssetsRootRejectsBrokenBundle(t *testing.T) {
	bundlePath := filepath.Join(t.TempDir(), "broken.spack")
	if err := os.WriteFile(bundlePath, []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := cmd.ValidateConfiguredAssetsRootForTest(bundlePath)
	if err == nil {
		t.Fatal("expected broken bundle to fail validation")
	}
	if !strings.Contains(err.Error(), "read assets.root bundle index") {
		t.Fatalf("expected bundle index error, got %v", err)
	}
}
