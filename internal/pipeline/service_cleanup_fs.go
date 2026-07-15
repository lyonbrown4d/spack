package pipeline

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

func removeCleanupFilePath(root, fullPath string) error {
	rootDir, relativePath, err := openCleanupRoot(root, fullPath)
	if err != nil {
		return err
	}
	defer discardCleanupRoot(rootDir)
	if err := rootDir.Remove(filepath.FromSlash(relativePath)); err != nil {
		return oops.Wrapf(err, "remove cleanup file")
	}
	return nil
}

func openCleanupRoot(root, fullPath string) (*os.Root, string, error) {
	cleanRoot, err := filepath.Abs(filepath.Clean(strings.TrimSpace(root)))
	if err != nil {
		return nil, "", oops.In("pipeline").Owner("cleanup").With("root", root).Wrap(err)
	}
	cleanPath, err := filepath.Abs(filepath.Clean(strings.TrimSpace(fullPath)))
	if err != nil {
		return nil, "", oops.In("pipeline").Owner("cleanup").With("path", fullPath).Wrap(err)
	}
	relativePath, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return nil, "", oops.In("pipeline").Owner("cleanup").With("path", fullPath).Wrap(err)
	}
	if cleanupRelativePathUnsafe(relativePath) {
		return nil, "", oops.In("pipeline").Owner("cleanup").With("path", fullPath).Wrap(errors.New("cleanup path escapes cache root"))
	}
	rootDir, err := openValidatedCleanupRoot(cleanRoot)
	if err != nil {
		return nil, "", err
	}
	return rootDir, filepath.ToSlash(relativePath), nil
}

func cleanupRelativePathUnsafe(relativePath string) bool {
	return relativePath == "." || filepath.IsAbs(relativePath) || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator))
}

func openValidatedCleanupRoot(root string) (*os.Root, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, oops.In("pipeline").Owner("cleanup").With("root", root).Wrap(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, oops.In("pipeline").Owner("cleanup").With("root", root).Wrap(errors.New("cleanup root symlink not allowed"))
	}
	if !info.IsDir() {
		return nil, oops.In("pipeline").Owner("cleanup").With("root", root).Wrap(errors.New("cleanup root is not a directory"))
	}
	rootDir, err := os.OpenRoot(root)
	if err != nil {
		return nil, oops.In("pipeline").Owner("cleanup").With("root", root).Wrap(err)
	}
	openedInfo, err := rootDir.Stat(".")
	if err != nil {
		discardCleanupRoot(rootDir)
		return nil, oops.In("pipeline").Owner("cleanup").With("root", root).Wrap(err)
	}
	if !os.SameFile(info, openedInfo) {
		discardCleanupRoot(rootDir)
		return nil, oops.In("pipeline").Owner("cleanup").With("root", root).Wrap(errors.New("cleanup root was replaced"))
	}
	return rootDir, nil
}

func discardCleanupRoot(root *os.Root) {
	if root == nil {
		return
	}
	if err := root.Close(); err != nil {
		return
	}
}
