// Package source provides asset source implementations.
package source

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/samber/oops"
)

var (
	// ErrSymlinkNotAllowed reports that a source path includes a symlink.
	ErrSymlinkNotAllowed = errors.New("source symlink not allowed")
	// ErrRootReplaced reports that the configured source root no longer refers to the original directory.
	ErrRootReplaced = errors.New("source root was replaced")
)

type LocalFS struct {
	root     string
	rootInfo fs.FileInfo
	logger   *slog.Logger
}

func NewLocalFS(cfg *config.Assets, logger *slog.Logger) (*LocalFS, error) {
	root := strings.TrimSpace(cfg.Root)
	if root == "" {
		return nil, oops.Owner("source").Wrap(errors.New("assets root is required"))
	}

	root, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, oops.Wrap(err)
	}

	info, err := os.Lstat(root)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	if isSymlink(info) {
		return nil, oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrSymlinkNotAllowed, root))
	}
	if !info.IsDir() {
		return nil, oops.Owner("source").Wrap(fmt.Errorf("assets root must be a directory: %s", root))
	}

	logger.Info("Source configured",
		slog.String("root", cfg.Root),
	)
	return &LocalFS{
		root:     root,
		rootInfo: info,
		logger:   logger,
	}, nil
}

func (s *LocalFS) Walk(walkFn func(File) error) error {
	if err := s.validateRoot(); err != nil {
		return err
	}
	if err := filepath.WalkDir(s.root, func(fullPath string, entry fs.DirEntry, err error) error {
		file, fileErr := buildWalkFile(s.root, fullPath, entry, err)
		if fileErr != nil {
			return fileErr
		}
		return walkFn(file)
	}); err != nil {
		return oops.Wrap(err)
	}
	return nil
}

func (s *LocalFS) FindFile(assetPath string) (File, bool, error) {
	relativePath, ok := cleanRelativeAssetPath(assetPath)
	if !ok {
		return File{}, false, nil
	}
	if err := s.validateRoot(); err != nil {
		return File{}, false, err
	}

	fullPath := filepath.Join(s.root, filepath.FromSlash(relativePath))
	if !isPathWithinRoot(s.root, fullPath) {
		return File{}, false, nil
	}

	info, err := s.lstatPathWithinRoot(fullPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{}, false, nil
		}
		return File{}, false, oops.Wrap(err)
	}
	if err := s.validateRoot(); err != nil {
		return File{}, false, err
	}

	return File{
		Path:     relativePath,
		FullPath: fullPath,
		Size:     info.Size(),
		IsDir:    info.IsDir(),
		ModTime:  info.ModTime(),
	}, true, nil
}

func buildWalkFile(root, fullPath string, entry fs.DirEntry, walkErr error) (File, error) {
	if walkErr != nil {
		return File{}, oops.Wrap(walkErr)
	}
	if entry.Type()&fs.ModeSymlink != 0 {
		return File{}, oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrSymlinkNotAllowed, fullPath))
	}

	info, err := entry.Info()
	if err != nil {
		return File{}, oops.Wrap(err)
	}
	rel, err := filepath.Rel(root, fullPath)
	if err != nil {
		return File{}, oops.Wrap(err)
	}

	return File{
		Path:     filepath.ToSlash(rel),
		FullPath: fullPath,
		Size:     info.Size(),
		IsDir:    entry.IsDir(),
		ModTime:  info.ModTime(),
	}, nil
}

func cleanRelativeAssetPath(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" ||
		strings.ContainsRune(trimmed, '\x00') ||
		strings.ContainsRune(trimmed, '\\') ||
		filepath.IsAbs(trimmed) ||
		path.IsAbs(trimmed) {
		return "", false
	}

	cleaned := path.Clean(trimmed)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return cleaned, true
}

func (s *LocalFS) validateRoot() error {
	info, err := os.Lstat(s.root)
	if err != nil {
		return oops.Wrap(err)
	}
	if isSymlink(info) {
		return oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrSymlinkNotAllowed, s.root))
	}
	if !info.IsDir() {
		return oops.Owner("source").Wrap(fmt.Errorf("assets root must be a directory: %s", s.root))
	}
	if s.rootInfo != nil && !os.SameFile(s.rootInfo, info) {
		return oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrRootReplaced, s.root))
	}
	return nil
}

func (s *LocalFS) lstatPathWithinRoot(fullPath string) (fs.FileInfo, error) {
	relativePath, err := filepath.Rel(s.root, fullPath)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	if relativePath == "." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) || relativePath == ".." {
		return nil, os.ErrNotExist
	}

	currentPath := s.root
	var info fs.FileInfo
	for segment := range strings.SplitSeq(relativePath, string(filepath.Separator)) {
		currentPath = filepath.Join(currentPath, segment)
		info, err = os.Lstat(currentPath)
		if err != nil {
			return nil, fmt.Errorf("stat source path %q: %w", currentPath, err)
		}
		if isSymlink(info) {
			return nil, oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrSymlinkNotAllowed, currentPath))
		}
	}
	return info, nil
}

func isPathWithinRoot(root, fullPath string) bool {
	rel, err := filepath.Rel(root, fullPath)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func isSymlink(info fs.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0
}
