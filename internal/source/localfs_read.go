package source

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/samber/oops"
)

func (s *LocalFS) ReadPrefix(assetPath string, maxBytes int64) ([]byte, bool, error) {
	relativePath, ok := readPrefixRelativePath(assetPath, maxBytes)
	if !ok {
		return nil, false, nil
	}

	fullPath := filepath.Join(s.root, filepath.FromSlash(relativePath))
	file, _, err := s.OpenFile(fullPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	body, readErr := io.ReadAll(io.LimitReader(file, maxBytes))
	return body, true, closeReadPrefixFile(file, readErr)
}

func readPrefixRelativePath(assetPath string, maxBytes int64) (string, bool) {
	if maxBytes <= 0 {
		return "", false
	}
	return cleanRelativeAssetPath(assetPath)
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
