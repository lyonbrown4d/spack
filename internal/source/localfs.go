// Package source provides asset source implementations.
package source

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/pkg"
	"github.com/samber/oops"
)

var (
	// ErrSymlinkNotAllowed reports that a source path includes a symlink.
	ErrSymlinkNotAllowed = errors.New("source symlink not allowed")
	// ErrRootReplaced reports that the configured source root no longer refers to the original directory.
	ErrRootReplaced     = errors.New("source root was replaced")
	errSourceContextNil = errors.New("source context is nil")
)

type LocalFS struct {
	root                     string
	rootInfo                 fs.FileInfo
	resources                *localFSResources
	logger                   *slog.Logger
	bundle                   *bundleSource
	bundleExtractionDuration time.Duration
}

func NewLocalFS(cfg *config.Assets, logger *slog.Logger) (*LocalFS, error) {
	return NewLocalFSContext(context.TODO(), cfg, logger)
}

func NewLocalFSContext(ctx context.Context, cfg *config.Assets, logger *slog.Logger) (*LocalFS, error) {
	return NewSourceFactory(NewResolver(), logger).LocalFSContext(ctx, cfg)
}

func (s *LocalFS) Cleanup() error {
	if s == nil || s.resources == nil {
		return nil
	}
	if err := s.resources.close(); err != nil {
		return oops.Owner("source").Wrap(err)
	}
	return nil
}

func (s *LocalFS) Walk(walkFn func(File) error) error {
	if err := s.validateRoot(); err != nil {
		return err
	}
	if s.bundle != nil {
		return s.walkBundle(walkFn)
	}
	files, err := s.walkDirectory()
	if err != nil {
		return err
	}
	for index := range files {
		file := files[index]
		if err := walkFn(file); err != nil {
			return err
		}
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
	if s.bundle != nil {
		return s.findBundleFile(relativePath)
	}

	var info fs.FileInfo
	err := s.resources.useRoot(func(rootDir *os.Root) error {
		var statErr error
		info, statErr = lstatPathWithinRoot(rootDir, s.root, relativePath)
		return statErr
	})
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
		FullPath: filepath.Join(s.root, filepath.FromSlash(relativePath)),
		Size:     info.Size(),
		IsDir:    info.IsDir(),
		ModTime:  info.ModTime(),
	}, true, nil
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
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || pkg.HasUnsafePortablePathSegment(cleaned) {
		return "", false
	}
	return cleaned, true
}

func lstatPathWithinRoot(rootDir *os.Root, root, relativePath string) (fs.FileInfo, error) {
	currentPath := ""
	var info fs.FileInfo
	for segment := range strings.SplitSeq(relativePath, "/") {
		if currentPath == "" {
			currentPath = segment
		} else {
			currentPath = path.Join(currentPath, segment)
		}
		var err error
		info, err = rootDir.Lstat(filepath.FromSlash(currentPath))
		if err != nil {
			return nil, oops.Wrapf(err, "stat source path %q", filepath.Join(root, filepath.FromSlash(currentPath)))
		}
		if isSymlink(info) {
			return nil, oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrSymlinkNotAllowed, filepath.Join(root, filepath.FromSlash(currentPath))))
		}
	}
	return info, nil
}

func isSymlink(info fs.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0
}
