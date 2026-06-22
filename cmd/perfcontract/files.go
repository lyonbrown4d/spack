package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

func readRepoFile(path string) []byte {
	clean, err := cleanRepoPath(path)
	if err != nil {
		fatalf("invalid input path %q: %v", path, err)
	}
	body, err := fs.ReadFile(os.DirFS("."), clean)
	if err != nil {
		fatalf("read %s: %v", clean, err)
	}
	return body
}

func readRepoDir(path string) ([]fs.DirEntry, bool) {
	clean, err := cleanRepoPath(path)
	if err != nil {
		fatalf("invalid input directory %q: %v", path, err)
	}
	entries, err := fs.ReadDir(os.DirFS("."), clean)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false
		}
		fatalf("read directory %s: %v", clean, err)
	}
	return entries, true
}

func repoFileExists(path string) bool {
	clean, err := cleanRepoPath(path)
	if err != nil {
		fatalf("invalid input path %q: %v", path, err)
	}
	_, err = fs.Stat(os.DirFS("."), clean)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	fatalf("stat %s: %v", clean, err)
	return false
}

func cleanRepoPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", oops.In("perfcontract").Owner("files").Wrap(errors.New("path is empty"))
	}
	if filepath.IsAbs(trimmed) {
		return "", oops.In("perfcontract").Owner("files").Wrap(errors.New("absolute paths are not allowed"))
	}
	clean := filepath.ToSlash(filepath.Clean(trimmed))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || !fs.ValidPath(clean) {
		return "", oops.In("perfcontract").Owner("files").Wrap(errors.New("path must stay inside the repository"))
	}
	return clean, nil
}

func writeFile(path string, body []byte) {
	clean, err := cleanRepoPath(path)
	if err != nil {
		fatalf("invalid output path %q: %v", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(clean), 0o750); err != nil {
		fatalf("create output directory: %v", err)
	}
	if err := os.WriteFile(clean, body, 0o600); err != nil {
		fatalf("write %s: %v", clean, err)
	}
}

func writeJSON(path string, value any) {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fatalf("encode metadata: %v", err)
	}
	writeFile(path, append(body, '\n'))
}
