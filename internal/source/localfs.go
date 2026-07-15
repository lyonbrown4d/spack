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
	"github.com/lyonbrown4d/spack/internal/spackbundle"
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
	logger                   *slog.Logger
	bundle                   *bundleSource
	cleanupRoot              string
	bundleExtractionDuration time.Duration
}

func NewLocalFS(cfg *config.Assets, logger *slog.Logger) (*LocalFS, error) {
	return NewLocalFSContext(context.TODO(), cfg, logger)
}

func NewLocalFSContext(ctx context.Context, cfg *config.Assets, logger *slog.Logger) (*LocalFS, error) {
	return NewSourceFactory(NewResolver(), logger).LocalFSContext(ctx, cfg)
}

func (s *LocalFS) Cleanup() error {
	if s == nil || strings.TrimSpace(s.cleanupRoot) == "" {
		return nil
	}
	extracted := spackbundle.Extracted{Root: s.cleanupRoot}
	if err := extracted.Cleanup(); err != nil {
		return oops.Owner("source").Wrap(err)
	}
	s.cleanupRoot = ""
	return nil
}

type resolvedLocalFSRoot struct {
	root                     string
	info                     fs.FileInfo
	bundle                   *bundleSource
	cleanupRoot              string
	bundleExtractionDuration time.Duration
}

func resolveLocalFSResolvedRoot(ctx context.Context, resolved Resolved) (resolvedLocalFSRoot, error) {
	switch resolved.Type {
	case TypeDirectory:
		return resolveLocalFSDirectoryRoot(resolved.Root)
	case TypeBundle:
		return resolveLocalFSBundleRoot(ctx, resolved.Root)
	default:
		return resolvedLocalFSRoot{}, oops.Owner("source").Wrap(fmt.Errorf("assets root must be a directory or .spack bundle: %s", resolved.Root))
	}
}

func resolveLocalFSBundleRoot(ctx context.Context, root string) (resolvedLocalFSRoot, error) {
	startedAt := time.Now()
	extracted, err := spackbundle.ExtractReadOnly(ctx, root)
	extractionDuration := time.Since(startedAt)
	if err != nil {
		return resolvedLocalFSRoot{}, oops.Owner("source").Wrap(err)
	}

	resolved, err := resolveLocalFSDirectoryRoot(extracted.Root)
	if err != nil {
		discardExtractedCleanup(extracted)
		return resolvedLocalFSRoot{}, err
	}
	bundle, err := newBundleSource(root, extracted.Root, extracted.Index)
	if err != nil {
		discardExtractedCleanup(extracted)
		return resolvedLocalFSRoot{}, oops.Owner("source").Wrap(err)
	}
	resolved.bundle = bundle
	resolved.cleanupRoot = extracted.Root
	resolved.bundleExtractionDuration = extractionDuration
	return resolved, nil
}

func discardExtractedCleanup(extracted spackbundle.Extracted) {
	if err := extracted.Cleanup(); err != nil {
		return
	}
}

func resolveLocalFSDirectoryRoot(root string) (resolvedLocalFSRoot, error) {
	rootDir, err := os.OpenRoot(root)
	if err != nil {
		return resolvedLocalFSRoot{}, oops.Wrap(err)
	}
	openedInfo, err := rootDir.Stat(".")
	if err != nil {
		closeRoot(rootDir)
		return resolvedLocalFSRoot{}, oops.Wrap(err)
	}
	currentInfo, err := os.Lstat(root)
	if err != nil {
		closeRoot(rootDir)
		return resolvedLocalFSRoot{}, oops.Wrap(err)
	}
	if err := validateOpenedDirectoryRoot(root, openedInfo, currentInfo); err != nil {
		closeRoot(rootDir)
		return resolvedLocalFSRoot{}, err
	}
	if err := rootDir.Close(); err != nil {
		return resolvedLocalFSRoot{}, oops.Wrap(err)
	}
	return resolvedLocalFSRoot{root: root, info: currentInfo}, nil
}

func validateOpenedDirectoryRoot(root string, openedInfo, currentInfo fs.FileInfo) error {
	if isSymlink(currentInfo) {
		return oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrSymlinkNotAllowed, root))
	}
	if !currentInfo.IsDir() {
		return oops.Owner("source").Wrap(fmt.Errorf("assets root must be a directory or .spack bundle: %s", root))
	}
	if !os.SameFile(openedInfo, currentInfo) {
		return oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrRootReplaced, root))
	}
	return nil
}

func closeRoot(rootDir *os.Root) {
	if err := rootDir.Close(); err != nil {
		return
	}
}

func logSourceConfigured(logger *slog.Logger, configuredRoot string, resolved resolvedLocalFSRoot) {
	if resolved.bundle == nil {
		logger.Info("Source configured",
			slog.String("root", configuredRoot),
		)
		return
	}
	logger.Info("Source bundle extracted",
		slog.String("bundle", configuredRoot),
		slog.String("root", resolved.root),
		slog.String("extract_root", resolved.cleanupRoot),
		slog.Int("files", resolved.bundle.entries.Len()),
		slog.Int64("bytes", bundleIndexBytes(resolved.bundle.index)),
		slog.Duration("duration", resolved.bundleExtractionDuration),
	)
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

	rootDir, err := s.openRoot()
	if err != nil {
		return File{}, false, err
	}
	defer closeRoot(rootDir)

	info, err := lstatPathWithinRoot(rootDir, s.root, relativePath)
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

func (s *LocalFS) openRoot() (*os.Root, error) {
	rootDir, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	return rootDir, nil
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
