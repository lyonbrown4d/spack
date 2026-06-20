package pipeline_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/lyonbrown4d/spack/internal/artifact"
)

func newTestStore(root string) artifact.Store {
	return &testStore{root: root}
}

type testStore struct {
	root string
}

func (s *testStore) Root() string {
	return s.root
}

func (s *testStore) PathFor(assetPath, sourceHash, namespace, suffix string) string {
	cleanPath := filepath.Clean(filepath.FromSlash(assetPath))
	return filepath.Join(s.root, namespace, sourceHash, cleanPath+suffix)
}

func (s *testStore) Write(path string, data []byte) error {
	// #nosec G703 -- test paths are created under t.TempDir().
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create test artifact directory: %w", err)
	}

	tmpPath := path + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	// #nosec G703 -- test paths are created under t.TempDir().
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write test artifact temp file: %w", err)
	}
	// #nosec G703 -- test paths are created under t.TempDir().
	if err := os.Rename(tmpPath, path); err != nil {
		// #nosec G703 -- test paths are created under t.TempDir().
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return errors.Join(fmt.Errorf("rename test artifact temp file: %w", err), fmt.Errorf("cleanup test artifact temp file: %w", removeErr))
		}
		return fmt.Errorf("rename test artifact temp file: %w", err)
	}
	return nil
}
