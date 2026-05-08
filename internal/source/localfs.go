// Package source provides asset source implementations.
package source

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/daiyuang/spack/internal/config"
	"github.com/samber/oops"
)

type localFS struct {
	root   string
	logger *slog.Logger
}

func newLocalFS(cfg *config.Assets, logger *slog.Logger) (Source, error) {
	root := strings.TrimSpace(cfg.Root)
	if root == "" {
		return nil, oops.Owner("source").Wrap(errors.New("assets root is required"))
	}

	info, err := os.Stat(root)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	if !info.IsDir() {
		return nil, oops.Owner("source").Wrap(fmt.Errorf("assets root must be a directory: %s", root))
	}

	logger.Info("Source configured",
		slog.String("backend", string(cfg.NormalizedBackend())),
		slog.String("root", cfg.Root),
	)
	return &localFS{
		root:   root,
		logger: logger,
	}, nil
}

func (s *localFS) Walk(walkFn func(File) error) error {
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

func (s *localFS) FindFile(path string) (File, bool, error) {
	trimmedPath := strings.TrimSpace(filepath.Clean(path))
	if trimmedPath == "" {
		return File{}, false, nil
	}

	fullPath := trimmedPath
	if !filepath.IsAbs(trimmedPath) {
		fullPath = filepath.Join(s.root, filepath.FromSlash(trimmedPath))
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return File{}, false, nil
		}
		return File{}, false, oops.Wrap(err)
	}

	relativePath, relErr := filepath.Rel(s.root, fullPath)
	if relErr != nil {
		return File{}, false, oops.Wrap(relErr)
	}
	relativePath = filepath.ToSlash(relativePath)
	if strings.HasPrefix(relativePath, "..") || relativePath == "." {
		return File{}, false, nil
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
