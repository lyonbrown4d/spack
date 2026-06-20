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
	"github.com/lyonbrown4d/spack/internal/spackbundle"
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
	bundle   *bundleSource
}

type bundleSource struct {
	path    string
	index   spackbundle.Index
	entries map[string]spackbundle.IndexFile
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

	resolved, err := resolveLocalFSRoot(root)
	if err != nil {
		return nil, err
	}

	logSourceConfigured(logger, cfg.Root, resolved)
	return &LocalFS{
		root:     resolved.root,
		rootInfo: resolved.info,
		logger:   logger,
		bundle:   resolved.bundle,
	}, nil
}

func (s *LocalFS) Cleanup() error {
	return nil
}

type resolvedLocalFSRoot struct {
	root   string
	info   fs.FileInfo
	bundle *bundleSource
}

func resolveLocalFSRoot(root string) (resolvedLocalFSRoot, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return resolvedLocalFSRoot{}, oops.Wrap(err)
	}
	if isSymlink(info) {
		return resolvedLocalFSRoot{}, oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrSymlinkNotAllowed, root))
	}
	if info.IsDir() {
		return resolvedLocalFSRoot{root: root, info: info}, nil
	}
	if info.Mode().IsRegular() && spackbundle.IsBundlePath(root) {
		bundle, err := newBundleSource(root)
		if err != nil {
			return resolvedLocalFSRoot{}, oops.Owner("source").Wrap(err)
		}
		return resolvedLocalFSRoot{root: root, info: info, bundle: bundle}, nil
	}
	return resolvedLocalFSRoot{}, oops.Owner("source").Wrap(fmt.Errorf("assets root must be a directory or .spack bundle: %s", root))
}

func newBundleSource(root string) (*bundleSource, error) {
	index, err := spackbundle.ReadIndex(root)
	if err != nil {
		return nil, fmt.Errorf("read source bundle index: %w", err)
	}
	entries := make(map[string]spackbundle.IndexFile, len(index.Files))
	for _, file := range index.Files {
		if strings.TrimSpace(file.Path) == "" {
			continue
		}
		entries[file.Path] = file
	}
	return &bundleSource{
		path:    root,
		index:   index,
		entries: entries,
	}, nil
}

func logSourceConfigured(logger *slog.Logger, configuredRoot string, resolved resolvedLocalFSRoot) {
	if resolved.bundle == nil {
		logger.Info("Source configured",
			slog.String("root", configuredRoot),
		)
		return
	}
	logger.Info("Source bundle configured",
		slog.String("bundle", configuredRoot),
		slog.Int("files", len(resolved.bundle.entries)),
	)
}

func (s *LocalFS) Walk(walkFn func(File) error) error {
	if err := s.validateRoot(); err != nil {
		return err
	}
	if s.bundle != nil {
		return s.walkBundle(walkFn)
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
	if s.bundle != nil {
		return s.findBundleFile(relativePath)
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

func (s *LocalFS) walkBundle(walkFn func(File) error) error {
	for _, entry := range s.bundle.index.Files {
		file, err := s.bundleFile(entry)
		if err != nil {
			return err
		}
		if err := walkFn(file); err != nil {
			return err
		}
	}
	return nil
}

func (s *LocalFS) findBundleFile(relativePath string) (File, bool, error) {
	entry, ok := s.bundle.entries[relativePath]
	if !ok {
		return File{}, false, nil
	}
	file, err := s.bundleFile(entry)
	if err != nil {
		return File{}, false, err
	}
	return file, true, nil
}

func (s *LocalFS) bundleFile(entry spackbundle.IndexFile) (File, error) {
	reference, err := spackbundle.NewReference(s.bundle.path, entry.Path)
	if err != nil {
		return File{}, oops.Wrap(err)
	}
	return File{
		Path:       entry.Path,
		FullPath:   reference,
		Size:       entry.Size,
		ModTime:    s.bundle.index.CreatedAt,
		MediaType:  entry.MediaType,
		SourceHash: entry.SourceHash,
		ETag:       entry.ETag,
	}, nil
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
