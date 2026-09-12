package pipeline

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/samber/oops"
)

func TestPipelineSourceFileCleansUpOwnedFallback(t *testing.T) {
	tests := []struct {
		name      string
		missing   bool
		operation func(*source.LocalFS, string, pipelineLocalDirectoryFactory) error
	}{
		{name: "read success", operation: readPipelineSourceFileForTest},
		{name: "read failure", missing: true, operation: readPipelineSourceFileForTest},
		{name: "validate success", operation: validatePipelineSourceFileForTest},
		{name: "validate failure", missing: true, operation: validatePipelineSourceFileForTest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertOwnedPipelineSourceCleanup(t, tt.missing, tt.operation)
		})
	}
}

func readPipelineSourceFileForTest(
	src *source.LocalFS,
	path string,
	factory pipelineLocalDirectoryFactory,
) error {
	_, err := readPipelineSourceFileWithFactory(src, path, factory)
	return err
}

func validatePipelineSourceFileForTest(
	src *source.LocalFS,
	path string,
	factory pipelineLocalDirectoryFactory,
) error {
	_, err := validatePipelineSourceFileWithFactory(src, path, factory)
	return err
}

func assertOwnedPipelineSourceCleanup(
	t *testing.T,
	missing bool,
	operation func(*source.LocalFS, string, pipelineLocalDirectoryFactory) error,
) {
	t.Helper()
	dir := t.TempDir()
	existingPath := filepath.Join(dir, "asset.txt")
	if err := os.WriteFile(existingPath, []byte("asset"), 0o600); err != nil {
		t.Fatal(err)
	}
	operationPath := existingPath
	if missing {
		operationPath = filepath.Join(dir, "missing.txt")
	}

	var fallback *source.LocalFS
	factory := func(root string) (*source.LocalFS, bool, error) {
		created, ok, createErr := source.NewLocalDirectory(root)
		if createErr != nil {
			return nil, false, oops.Wrapf(createErr, "create test fallback source")
		}
		fallback = created
		return fallback, ok, nil
	}
	operationErr := operation(nil, operationPath, factory)
	if missing && operationErr == nil {
		t.Fatal("operation error = nil, want missing file error")
	}
	if !missing && operationErr != nil {
		t.Fatalf("operation error = %v", operationErr)
	}
	if fallback == nil {
		t.Fatal("fallback source was not created")
	}
	if _, readErr := fallback.ReadFile(existingPath); !errors.Is(readErr, fs.ErrClosed) {
		t.Fatalf("fallback read error = %v, want fs.ErrClosed", readErr)
	}
}

func TestPipelineSourceFileLeavesBorrowedSourceOpen(t *testing.T) {
	tests := []struct {
		name      string
		operation func(*source.LocalFS, string) error
	}{
		{name: "read", operation: readBorrowedPipelineSourceFileForTest},
		{name: "validate", operation: validateBorrowedPipelineSourceFileForTest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertBorrowedPipelineSourceRemainsOpen(t, tt.operation)
		})
	}
}

func readBorrowedPipelineSourceFileForTest(src *source.LocalFS, path string) error {
	_, err := readPipelineSourceFile(src, path)
	return err
}

func validateBorrowedPipelineSourceFileForTest(src *source.LocalFS, path string) error {
	_, err := validatePipelineSourceFile(src, path)
	return err
}

func assertBorrowedPipelineSourceRemainsOpen(
	t *testing.T,
	operation func(*source.LocalFS, string) error,
) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "asset.txt")
	want := []byte("asset")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	src, ok, createErr := source.NewLocalDirectory(dir)
	if createErr != nil {
		t.Fatal(createErr)
	}
	if !ok || src == nil {
		t.Fatal("borrowed source was not created")
	}
	t.Cleanup(func() {
		if cleanupErr := src.Cleanup(); cleanupErr != nil {
			t.Errorf("cleanup borrowed source: %v", cleanupErr)
		}
	})

	if operationErr := operation(src, path); operationErr != nil {
		t.Fatalf("operation error = %v", operationErr)
	}
	body, readErr := src.ReadFile(path)
	if readErr != nil {
		t.Fatalf("borrowed source was closed: %v", readErr)
	}
	if !bytes.Equal(body, want) {
		t.Fatalf("borrowed source body = %q, want %q", body, want)
	}
}

func TestJoinPipelineSourceCleanupErrorPreservesBothErrors(t *testing.T) {
	operationErr := errors.New("operation failed")
	cleanupErr := errors.New("cleanup failed")

	err := joinPipelineSourceCleanupError(operationErr, cleanupErr)
	if !errors.Is(err, operationErr) {
		t.Fatalf("joined error does not contain operation error: %v", err)
	}
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("joined error does not contain cleanup error: %v", err)
	}
}
