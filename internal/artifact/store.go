package artifact

import (
	"errors"
	"fmt"
	"github.com/samber/oops"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Store interface {
	Root() string
	PathFor(assetPath, sourceHash, namespace, suffix string) string
	Write(path string, data []byte) error
}

type LocalStore struct {
	root string
}

func newLocalStore(root string) (Store, error) {
	normalizedRoot := strings.TrimSpace(root)
	if normalizedRoot == "" {
		return nil, oops.In("artifact").Owner("store").Wrap(errors.New("missing cache root"))
	}
	if err := os.MkdirAll(normalizedRoot, 0o750); err != nil {
		return nil, oops.In("artifact").Owner("store").With("root", normalizedRoot).Wrap(err)
	}
	info, err := os.Stat(normalizedRoot)
	if err != nil {
		return nil, oops.In("artifact").Owner("store").With("root", normalizedRoot).Wrap(err)
	}
	if !info.IsDir() {
		return nil, oops.In("artifact").Owner("store").With("root", normalizedRoot).Wrap(fmt.Errorf("cache root is not a directory: %s", normalizedRoot))
	}
	return &LocalStore{root: normalizedRoot}, nil
}

func (s *LocalStore) Root() string {
	return s.root
}

func (s *LocalStore) PathFor(assetPath, sourceHash, namespace, suffix string) string {
	cleanPath := filepath.FromSlash(strings.TrimPrefix(assetPath, "/"))
	cleanPath = filepath.Clean(cleanPath)
	if cleanPath == "." {
		cleanPath = "index"
	}

	return filepath.Join(s.root, namespace, sourceHash, cleanPath+suffix)
}

func (s *LocalStore) Write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create artifact directory: %w", err)
	}

	tmpPath := path + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write artifact temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return errors.Join(fmt.Errorf("rename artifact temp file: %w", err), fmt.Errorf("cleanup artifact temp file: %w", removeErr))
		}
		return fmt.Errorf("rename artifact temp file: %w", err)
	}

	return nil
}
