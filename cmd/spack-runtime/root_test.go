package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeRootCommandDoesNotExposeCompile(t *testing.T) {
	root, err := newRootCommand()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := root.Find([]string{"compile"}); err == nil {
		t.Fatal("expected runtime command tree to exclude compile")
	}
}

func TestRuntimeUtilityCommandsRunThroughUtilityLifecycle(t *testing.T) {
	assets := writeRuntimeCommandAssets(t)
	tests := []struct {
		name string
		args []string
	}{
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
			command, err := newRootCommand()
			if err != nil {
				t.Fatal(err)
			}
			command.SetOut(io.Discard)
			command.SetErr(io.Discard)
			command.SetArgs(test.args)
			if err := command.ExecuteContext(context.Background()); err != nil {
				t.Fatalf("execute %s through utility lifecycle: %v", test.name, err)
			}
		})
	}
}

func writeRuntimeCommandAssets(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<h1>ok</h1>"), 0o600); err != nil {
		t.Fatalf("write runtime command asset: %v", err)
	}
	return root
}
