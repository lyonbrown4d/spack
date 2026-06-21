package main

import "testing"

func TestCompilerRootCommandDoesNotExposeHealthcheck(t *testing.T) {
	root := newRootCommand()
	if _, _, err := root.Find([]string{"healthcheck"}); err == nil {
		t.Fatal("expected compiler command tree to exclude healthcheck")
	}
	if _, _, err := root.Find([]string{"compile"}); err != nil {
		t.Fatalf("expected compiler command tree to include compile: %v", err)
	}
}
