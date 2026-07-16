package source

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

// ReadFile reads a file only after binding the open to this source's root.
func (s *LocalFS) ReadFile(fullPath string) ([]byte, error) {
	file, _, err := s.OpenFile(fullPath)
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil {
		return nil, oops.Wrap(readErr)
	}
	if closeErr != nil {
		return nil, oops.Wrap(closeErr)
	}
	return body, nil
}

// OpenFile opens a regular file under this source root while rejecting
// symlinks, root replacement, path escapes, and open-time replacement.
func (s *LocalFS) OpenFile(fullPath string) (*os.File, fs.FileInfo, error) {
	if s == nil {
		return nil, nil, oops.Owner("source").Wrap(errors.New("local source is nil"))
	}
	relativePath, err := s.relativePath(fullPath)
	if err != nil {
		return nil, nil, err
	}
	rootDir, err := s.openValidatedRoot()
	if err != nil {
		return nil, nil, err
	}
	defer closeRoot(rootDir)
	return s.openStableFile(rootDir, relativePath, fullPath)
}

func (s *LocalFS) openValidatedRoot() (*os.Root, error) {
	rootDir, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	if err := s.validateCurrentRoot(rootDir); err != nil {
		closeRoot(rootDir)
		return nil, err
	}
	return rootDir, nil
}

func (s *LocalFS) openStableFile(rootDir *os.Root, relativePath, fullPath string) (*os.File, fs.FileInfo, error) {
	info, err := s.lstatRegularFile(rootDir, relativePath, fullPath)
	if err != nil {
		return nil, nil, err
	}
	file, err := rootDir.Open(filepath.FromSlash(relativePath))
	if err != nil {
		return nil, nil, oops.Wrap(err)
	}
	openedInfo, err := file.Stat()
	if err != nil {
		discardClose(file)
		return nil, nil, oops.Wrap(err)
	}
	if openedInfo.IsDir() {
		discardClose(file)
		return nil, nil, oops.Owner("source").Wrap(fmt.Errorf("source path is a directory: %s", fullPath))
	}
	if !os.SameFile(info, openedInfo) {
		discardClose(file)
		return nil, nil, oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrRootReplaced, fullPath))
	}
	return file, openedInfo, nil
}

func (s *LocalFS) lstatRegularFile(rootDir *os.Root, relativePath, fullPath string) (fs.FileInfo, error) {
	info, err := lstatPathWithinRoot(rootDir, s.root, relativePath)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	if info.IsDir() {
		return nil, oops.Owner("source").Wrap(fmt.Errorf("source path is a directory: %s", fullPath))
	}
	return info, nil
}

func (s *LocalFS) validateCurrentRoot(rootDir *os.Root) error {
	openedInfo, err := rootDir.Stat(".")
	if err != nil {
		return oops.Wrap(err)
	}
	currentInfo, err := os.Lstat(s.root)
	if err != nil {
		return oops.Wrap(err)
	}
	if err := validateOpenedDirectoryRoot(s.root, openedInfo, currentInfo); err != nil {
		return err
	}
	if !os.SameFile(s.rootInfo, currentInfo) {
		return oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrRootReplaced, s.root))
	}
	return nil
}

func (s *LocalFS) relativePath(fullPath string) (string, error) {
	absolute, err := filepath.Abs(filepath.Clean(strings.TrimSpace(fullPath)))
	if err != nil {
		return "", oops.Wrapf(err, "resolve source path")
	}
	relativePath, err := filepath.Rel(s.root, absolute)
	if err != nil {
		return "", oops.Wrapf(err, "resolve source relative path")
	}
	if relativePath == "." || filepath.IsAbs(relativePath) || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", oops.Owner("source").Wrap(fmt.Errorf("source path escaped root: %s", fullPath))
	}
	cleaned, ok := cleanRelativeAssetPath(filepath.ToSlash(relativePath))
	if !ok {
		return "", oops.Owner("source").Wrap(fmt.Errorf("invalid source path: %s", fullPath))
	}
	return cleaned, nil
}

func discardClose(file *os.File) {
	if file == nil {
		return
	}
	if err := file.Close(); err != nil {
		return
	}
}
