package main

import "testing"

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
