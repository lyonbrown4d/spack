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

	var file *os.File
	var info fs.FileInfo
	err = s.resources.useRoot(func(rootDir *os.Root) error {
		if validateErr := s.validateCurrentRoot(rootDir); validateErr != nil {
			return validateErr
		}
		openedFile, openedInfo, openErr := s.openStableFile(rootDir, relativePath, fullPath)
		if openErr != nil {
			return openErr
		}
		if validateErr := s.validateCurrentRoot(rootDir); validateErr != nil {
			discardClose(openedFile)
			return validateErr
		}
		file = openedFile
		info = openedInfo
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return file, info, nil
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
	currentInfo, err := s.lstatRegularFile(rootDir, relativePath, fullPath)
	if err != nil {
		discardClose(file)
		return nil, nil, err
	}
	if !os.SameFile(info, openedInfo) || !os.SameFile(currentInfo, openedInfo) {
		discardClose(file)
		return nil, nil, oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrRootReplaced, fullPath))
	}
	return file, openedInfo, nil
}

// TrustedReadOnlyPath returns a root-bound read-only filesystem only for
// immutable files extracted from a validated bundle.
func (s *LocalFS) TrustedReadOnlyPath(fullPath string) (fs.FS, string, bool, error) {
	if s == nil {
		return nil, "", false, oops.Owner("source").Wrap(errors.New("local source is nil"))
	}
	if s.bundle == nil {
		return nil, "", false, nil
	}
	relativePath, err := s.relativePath(fullPath)
	if err != nil {
		return nil, "", false, err
	}
	if _, ok := s.bundle.entries.GetOption(relativePath).Get(); !ok {
		return nil, "", false, nil
	}
	err = s.resources.useRoot(func(rootDir *os.Root) error {
		if validateErr := s.validateCurrentRoot(rootDir); validateErr != nil {
			return validateErr
		}
		if _, statErr := s.lstatRegularFile(rootDir, relativePath, fullPath); statErr != nil {
			return statErr
		}
		return s.validateCurrentRoot(rootDir)
	})
	if err != nil {
		return nil, "", false, err
	}
	return &localFSReadOnly{resources: s.resources}, relativePath, true, nil
}

type localFSReadOnly struct {
	resources *localFSResources
}

func (f *localFSReadOnly) Open(name string) (fs.File, error) {
	var file fs.File
	err := f.resources.useRoot(func(rootDir *os.Root) error {
		var openErr error
		file, openErr = rootDir.FS().Open(name)
		if openErr != nil {
			return oops.Wrap(openErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return file, nil
}

func (s *LocalFS) lstatRegularFile(rootDir *os.Root, relativePath, fullPath string) (fs.FileInfo, error) {
	info, err := lstatPathWithinRoot(rootDir, s.root, relativePath)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	if info.IsDir() {
		return nil, oops.Owner("source").Wrap(fmt.Errorf("source path is a directory: %s", fullPath))
	}
	if !info.Mode().IsRegular() {
		return nil, oops.Owner("source").Wrap(fmt.Errorf("source path is not a regular file: %s", fullPath))
	}
	return info, nil
}

func (s *LocalFS) validateCurrentRoot(rootDir *os.Root) error {
	return validateCurrentLocalFSRoot(rootDir, s.root, s.rootInfo)
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
