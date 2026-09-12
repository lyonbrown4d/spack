package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCompilerRootCommandDoesNotExposeHealthcheck(t *testing.T) {
	root := newRootCommand()
	if _, _, err := root.Find([]string{"healthcheck"}); err == nil {
		t.Fatal("expected compiler command tree to exclude healthcheck")
	}
	for _, name := range []string{"compile", "verify", "decompile"} {
		if _, _, err := root.Find([]string{name}); err != nil {
			t.Fatalf("expected compiler command tree to include %s: %v", name, err)
		}
	}
}

func TestCompilerUtilityCommandsRunThroughUtilityLifecycle(t *testing.T) {
	assets := writeCompilerCommandAssets(t)
	output := filepath.Join(t.TempDir(), "app.spack")
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "compile",
			args: []string{
				"compile", assets,
				"--output", output,
				"--compression.enable=false",
				"--image.enable=false",
			},
		},
		{
			name: "config validate",
			args: []string{"config", "validate", "--assets.root", assets},
		},
		{
			name: "config print-effective",
			args: []string{"config", "print-effective", "--assets.root", assets, "--redact"},
		},
		{
			name: "inspect",
			args: []string{"inspect", "--assets", assets, "--image.enable=false"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := newRootCommand()
			command.SetOut(io.Discard)
			command.SetErr(io.Discard)
			command.SetArgs(test.args)
			if err := command.ExecuteContext(context.Background()); err != nil {
				t.Fatalf("execute %s through utility lifecycle: %v", test.name, err)
			}
		})
	}

	if _, err := os.Stat(output); err != nil {
		t.Fatalf("stat compiled bundle: %v", err)
	}
}

func writeCompilerCommandAssets(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<h1>ok</h1>"), 0o600); err != nil {
		t.Fatalf("write compiler command asset: %v", err)
	}
	return root
}
