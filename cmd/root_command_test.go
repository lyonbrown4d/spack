package cmd_test

import (
	"testing"

	"github.com/lyonbrown4d/spack/cmd"
)

func TestRuntimeRootCommandDoesNotExposeCompile(t *testing.T) {
	root := cmd.NewRuntimeRootCommandForTest()
	if _, _, err := root.Find([]string{"compile"}); err == nil {
		t.Fatal("expected runtime command tree to exclude compile")
	}
}

func TestCompilerRootCommandDoesNotExposeHealthcheck(t *testing.T) {
	root := cmd.NewCompilerRootCommandForTest()
	if _, _, err := root.Find([]string{"healthcheck"}); err == nil {
		t.Fatal("expected compiler command tree to exclude healthcheck")
	}
	if _, _, err := root.Find([]string{"compile"}); err != nil {
		t.Fatalf("expected compiler command tree to include compile: %v", err)
	}
}
