package source

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/samber/oops"
)

func (s *LocalFS) ReadPrefix(assetPath string, maxBytes int64) ([]byte, bool, error) {
	relativePath, ok := readPrefixRelativePath(assetPath, maxBytes)
	if !ok {
		return nil, false, nil
	}

	rootDir, err := s.openValidatedRoot()
	if err != nil {
		return nil, false, err
	}
	defer closeRoot(rootDir)

	found, err := s.hasReadableRegularFile(rootDir, relativePath)
	if err != nil || !found {
		return nil, found, err
	}

	body, err := readRootFilePrefix(rootDir, relativePath, maxBytes)
	if err != nil {
		return nil, false, err
	}
	if err := s.validateRoot(); err != nil {
		return nil, false, err
	}
	return body, true, nil
}

func readPrefixRelativePath(assetPath string, maxBytes int64) (string, bool) {
	if maxBytes <= 0 {
		return "", false
	}
	return cleanRelativeAssetPath(assetPath)
}

func (s *LocalFS) openValidatedRoot() (*os.Root, error) {
	if err := s.validateRoot(); err != nil {
		return nil, err
	}
	return s.openRoot()
}

func (s *LocalFS) hasReadableRegularFile(rootDir *os.Root, relativePath string) (bool, error) {
	info, err := lstatPathWithinRoot(rootDir, s.root, relativePath)
	if err != nil {
		return false, readPrefixStatError(err)
	}
	return !info.IsDir(), nil
}

func readPrefixStatError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return oops.Wrap(err)
}

func readRootFilePrefix(rootDir *os.Root, relativePath string, maxBytes int64) ([]byte, error) {
	file, err := rootDir.Open(filepath.FromSlash(relativePath))
	if err != nil {
		return nil, readPrefixOpenError(err)
	}
	body, readErr := io.ReadAll(io.LimitReader(file, maxBytes))
	return body, closeReadPrefixFile(file, readErr)
}

func readPrefixOpenError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return oops.Wrap(err)
}

func closeReadPrefixFile(file *os.File, readErr error) error {
	closeErr := file.Close()
	if readErr != nil {
		return oops.Wrap(readErr)
	}
	if closeErr != nil {
		return oops.Wrap(closeErr)
	}
	return nil
}
