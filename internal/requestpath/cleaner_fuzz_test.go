package requestpath_test

import (
	"path"
	"strings"
	"testing"

	"github.com/lyonbrown4d/spack/internal/requestpath"
)

func FuzzCleanDoesNotProduceEscapingPath(f *testing.F) {
	for _, seed := range []string{
		"",
		"/",
		"app.js",
		"/assets/app.js",
		"../secret.txt",
		"/assets/%2e%2e/secret.txt",
		"/assets/%2e%2e%5csecret.txt",
		"%252e%252e%255csecret.txt",
		`..\secret.txt`,
		`\\server\share\asset.js`,
		`C:\Windows\win.ini`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 4096 {
			t.Skip("path seed is larger than the HTTP path budget")
		}

		cleaned := requestpath.Clean(raw)
		assertCleanedPathSafe(t, raw, cleaned.Value)
	})
}

func assertCleanedPathSafe(t *testing.T, raw, value string) {
	t.Helper()
	if value == "" {
		return
	}
	if strings.HasPrefix(value, "/") {
		t.Fatalf("cleaned path must be relative, got %q from %q", value, raw)
	}
	if strings.ContainsRune(value, '\\') {
		t.Fatalf("cleaned path must not retain backslash separators, got %q from %q", value, raw)
	}
	if containsTraversalSegment(value) {
		t.Fatalf("cleaned path must not contain traversal segment, got %q from %q", value, raw)
	}
}

func containsTraversalSegment(value string) bool {
	for segment := range strings.SplitSeq(path.Clean(value), "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}
