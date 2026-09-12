package source

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/oops"
)

type localFSResources struct {
	mu      sync.RWMutex
	state   *localFSResourceState
	cleanup runtime.Cleanup
	closed  bool
}

type localFSResourceState struct {
	once      sync.Once
	root      string
	rootInfo  fs.FileInfo
	rootDir   *os.Root
	extracted spackbundle.Extracted
	err       error
}

func newLocalFSResources(
	root string,
	rootInfo fs.FileInfo,
	rootDir *os.Root,
	extracted spackbundle.Extracted,
) *localFSResources {
	state := &localFSResourceState{
		root:      root,
		rootInfo:  rootInfo,
		rootDir:   rootDir,
		extracted: extracted,
	}
	resources := &localFSResources{state: state}
	resources.cleanup = runtime.AddCleanup(resources, cleanupLocalFSResourceState, state)
	return resources
}

func cleanupLocalFSResourceState(state *localFSResourceState) {
	if state == nil {
		return
	}
	state.close()
}

func (r *localFSResourceState) close() {
	r.once.Do(func() {
		var errs []error
		if err := r.closeRoot(); err != nil {
			errs = append(errs, oops.Wrap(err))
		}
		if err := r.extracted.Cleanup(); err != nil {
			errs = append(errs, oops.Wrap(err))
		}
		r.err = errors.Join(errs...)
	})
}

func (r *localFSResources) useRoot(use func(*os.Root) error) error {
	if r == nil {
		return fs.ErrClosed
	}
	r.mu.RLock()
	defer func() {
		r.mu.RUnlock()
		runtime.KeepAlive(r)
	}()
	if r.closed || r.state == nil {
		return fs.ErrClosed
	}
	return r.state.useRoot(use)
}

func (r *localFSResources) close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer func() {
		r.mu.Unlock()
		runtime.KeepAlive(r)
	}()
	r.cleanup.Stop()
	r.closed = true
	if r.state == nil {
		return nil
	}
	r.state.close()
	return r.state.err
}

type resolvedLocalFSRoot struct {
	root                     string
	info                     fs.FileInfo
	rootDir                  *os.Root
	bundle                   *bundleSource
	cleanupRoot              string
	extracted                spackbundle.Extracted
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
		closeRoot(resolved.rootDir)
		discardExtractedCleanup(extracted)
		return resolvedLocalFSRoot{}, oops.Owner("source").Wrap(err)
	}
	resolved.bundle = bundle
	resolved.cleanupRoot = extracted.Root
	resolved.extracted = extracted
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
	validationErr := validateOpenedDirectoryRoot(root, openedInfo, currentInfo)
	if validationErr != nil {
		closeRoot(rootDir)
		return resolvedLocalFSRoot{}, validationErr
	}
	if err := prepareLocalFSRoot(&rootDir); err != nil {
		return resolvedLocalFSRoot{}, err
	}
	return resolvedLocalFSRoot{root: root, info: currentInfo, rootDir: rootDir}, nil
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

func validateCurrentLocalFSRoot(rootDir *os.Root, root string, rootInfo fs.FileInfo) error {
	openedInfo, err := rootDir.Stat(".")
	if err != nil {
		return oops.Wrap(err)
	}
	currentInfo, err := os.Lstat(root)
	if err != nil {
		return oops.Wrap(err)
	}
	if err := validateOpenedDirectoryRoot(root, openedInfo, currentInfo); err != nil {
		return err
	}
	if !os.SameFile(rootInfo, currentInfo) {
		return oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrRootReplaced, root))
	}
	return nil
}

func closeRoot(rootDir *os.Root) {
	if rootDir == nil {
		return
	}
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
