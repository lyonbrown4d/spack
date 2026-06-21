package main

import "testing"

func TestRuntimeRootCommandDoesNotExposeCompile(t *testing.T) {
	root := newRootCommand()
	if _, _, err := root.Find([]string{"compile"}); err == nil {
		t.Fatal("expected runtime command tree to exclude compile")
	}
}
