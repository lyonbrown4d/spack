package spackbundle

import (
	"errors"
	"io/fs"
	"os"
	"slices"

	"github.com/samber/oops"
)

const (
	extractedReadOnlyDirMode  fs.FileMode = 0o555
	extractedReadOnlyFileMode fs.FileMode = 0o444
	extractedWritableDirMode  fs.FileMode = 0o755
	extractedWritableFileMode fs.FileMode = 0o644
)

func makeExtractedTreeReadOnly(root string) error {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return oops.Wrapf(err, "open extracted root")
	}
	defer func() {
		discardError(rootHandle.Close())
	}()

	dirs, err := makeExtractedFilesReadOnly(rootHandle)
	if err != nil {
		return err
	}
	return makeExtractedDirsReadOnly(rootHandle, dirs)
}

func makeExtractedFilesReadOnly(rootHandle *os.Root) ([]string, error) {
	var dirs []string
	if err := fs.WalkDir(rootHandle.FS(), ".", func(relativePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			dirs = append(dirs, relativePath)
			return nil
		}
		if err := rootHandle.Chmod(relativePath, extractedReadOnlyFileMode); err != nil {
			return oops.Wrapf(err, "make extracted file read-only %q", relativePath)
		}
		return nil
	}); err != nil {
		return nil, oops.Wrapf(err, "make extracted bundle files read-only")
	}
	return dirs, nil
}

func makeExtractedDirsReadOnly(rootHandle *os.Root, dirs []string) error {
	for _, dir := range slices.Backward(dirs) {
		if err := rootHandle.Chmod(dir, extractedReadOnlyDirMode); err != nil {
			return oops.Wrapf(err, "make extracted directory read-only %q", dir)
		}
	}
	return nil
}

func makeExtractedTreeWritable(root string) error {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return oops.Wrapf(err, "open extracted root")
	}
	defer func() {
		discardError(rootHandle.Close())
	}()

	if err := fs.WalkDir(rootHandle.FS(), ".", func(relativePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		mode := extractedWritableFileMode
		if entry.IsDir() {
			mode = extractedWritableDirMode
		}
		if err := rootHandle.Chmod(relativePath, mode); err != nil && !errors.Is(err, os.ErrNotExist) {
			return oops.Wrapf(err, "make extracted path writable %q", relativePath)
		}
		return nil
	}); err != nil {
		return oops.Wrapf(err, "make extracted bundle writable")
	}
	return nil
}
