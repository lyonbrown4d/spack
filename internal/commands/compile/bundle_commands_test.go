package compilecmd_test

import (
	"bytes"

	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	compilecmd "github.com/lyonbrown4d/spack/internal/commands/compile"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

func TestVerifyCommandRunsBundleVerification(t *testing.T) {
	bundle := writeBundleCommandTestBundle(t)
	command := compilecmd.NewVerifyCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{bundle})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); !strings.Contains(got, "verified "+bundle) {
		t.Fatalf("expected success output, got %q", got)
	}
}

func TestDecompileCommandRequiresOutput(t *testing.T) {
	command := compilecmd.NewDecompileCommand()
	command.SilenceUsage = true
	command.SetArgs([]string{"app.spack"})

	err := command.Execute()
	if err == nil {
		t.Fatal("expected missing output error")
	}
	if !strings.Contains(err.Error(), "output directory is required") {
		t.Fatalf("expected missing output error, got %v", err)
	}
}

func TestDecompileCommandRunsBundleDecompile(t *testing.T) {
	bundle := writeBundleCommandTestBundle(t)
	outDir := filepath.Join(t.TempDir(), "out")
	command := compilecmd.NewDecompileCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{bundle, "-o", outDir})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); !strings.Contains(got, "decompiled "+bundle+" into "+outDir) {
		t.Fatalf("expected success output, got %q", got)
	}
	body := readDecompiledIndex(t, outDir)
	if string(body) != "<h1>ok</h1>" {
		t.Fatalf("unexpected decompiled body %q", body)
	}
}

func readDecompiledIndex(t *testing.T, outDir string) []byte {
	t.Helper()
	root, err := os.OpenRoot(outDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := root.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}()
	file, err := root.Open("index.html")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}()
	body, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func writeBundleCommandTestBundle(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	asset := filepath.Join(root, "index.html")
	if err := os.WriteFile(asset, []byte("<h1>ok</h1>"), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(t.TempDir(), "app.spack")
	if _, err := spackbundle.Write(t.Context(), spackbundle.WriteOptions{
		Output: bundle,
		Root:   root,
		Files: []spackbundle.File{
			{Path: "index.html", FullPath: asset, Kind: "asset", MediaType: "text/html"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return bundle
}
