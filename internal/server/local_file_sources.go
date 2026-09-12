package server

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/samber/oops"
)

type serverFileSource struct {
	files *source.LocalFS
	owned bool
}

type serverFileSources struct {
	sources        atomic.Pointer[cxlist.List[serverFileSource]]
	sourcesMu      sync.Mutex
	trustedRoots   map[*source.LocalFS]fs.FS
	trustedRootsMu sync.Mutex
	cleanupOnce    sync.Once
	cleanupErrMu   sync.Mutex
	cleanupErr     error
	closed         bool
}

func newServerFileSources(
	cfg *config.Config,
	src *source.LocalFS,
	cat catalog.Catalog,
	logger *slog.Logger,
) *serverFileSources {
	return mergeServerFileSources(
		newServerFileSourcesFromSource(src),
		newServerFileSourcesFromConfig(cfg, logger),
		newServerFileSourcesFromCatalog(cat, logger),
	)
}

func newServerFileSourcesFromSource(src *source.LocalFS) *serverFileSources {
	if src != nil && src.Root() != "" {
		return newServerFileSourcesWithEntries(serverFileSource{files: src})
	}
	return nil
}

func newServerFileSourcesFromConfig(cfg *config.Config, logger *slog.Logger) *serverFileSources {
	if cfg == nil {
		return nil
	}
	return mergeServerFileSources(
		newServerFileSourcesFromRoot(cfg.Assets.Root, logger),
		newServerFileSourcesFromOptionalRoot(cfg.Compression.CacheDir, logger),
	)
}

func newServerFileSourcesFromCatalog(cat catalog.Catalog, logger *slog.Logger) *serverFileSources {
	if cat == nil {
		return nil
	}
	return newServerFileSourcesFromRoot(catalogFileSourceRoot(cat), logger)
}

func newServerFileSourcesFromRoot(root string, logger *slog.Logger) *serverFileSources {
	files := serverFileSourceFromRoot(root, logger)
	if files == nil {
		return nil
	}
	return newServerFileSourcesWithEntries(serverFileSource{files: files, owned: true})
}

func newServerFileSourcesFromOptionalRoot(root string, logger *slog.Logger) *serverFileSources {
	files := serverFileSourceFromOptionalRoot(root, logger)
	if files == nil {
		return nil
	}
	return newServerFileSourcesWithEntries(serverFileSource{files: files, owned: true})
}

func refreshServerFileSources(
	current *serverFileSources,
	cfg *config.Config,
	cat catalog.Catalog,
	logger *slog.Logger,
) *serverFileSources {
	if current == nil {
		current = newServerFileSourcesWithEntries()
	}
	if cfg != nil {
		current.addOwnedRoot(cfg.Assets.Root, false, logger)
		current.addOwnedRoot(cfg.Compression.CacheDir, true, logger)
	}
	if cat != nil {
		current.addOwnedRoot(catalogFileSourceRoot(cat), false, logger)
	}
	if current.empty() {
		return nil
	}
	return current
}

func (s *serverFileSources) addOwnedRoot(root string, optional bool, logger *slog.Logger) {
	if s == nil || root == "" || s.containsRoot(root) {
		return
	}

	var files *source.LocalFS
	if optional {
		files = serverFileSourceFromOptionalRoot(root, logger)
	} else {
		files = serverFileSourceFromRoot(root, logger)
	}
	if files == nil {
		return
	}

	entry := serverFileSource{files: files, owned: true}
	discard := false
	s.sourcesMu.Lock()
	current := s.sources.Load()
	if s.closed || serverFileSourceListContainsRoot(current, root) {
		discard = true
	} else {
		next := cxlist.NewList[serverFileSource]()
		if current != nil {
			next = current.Clone()
		}
		next.Add(entry)
		s.sources.Store(next)
	}
	s.sourcesMu.Unlock()

	if discard {
		s.discardOwned(entry)
	}
}

func (s *serverFileSources) containsRoot(root string) bool {
	if s == nil {
		return false
	}
	return serverFileSourceListContainsRoot(s.sources.Load(), root)
}

func serverFileSourceListContainsRoot(sources *cxlist.List[serverFileSource], root string) bool {
	if sources == nil {
		return false
	}
	key := serverFileSourceKey(root)
	if key == "" {
		return false
	}

	found := false
	sources.Range(func(_ int, entry serverFileSource) bool {
		if entry.files == nil || serverFileSourceKey(entry.files.Root()) != key {
			return true
		}
		found = true
		return false
	})
	return found
}
func serverFileSourceKey(root string) string {
	if root == "" {
		return ""
	}
	cleaned, err := filepath.Abs(root)
	if err != nil {
		cleaned = filepath.Clean(root)
	}
	if runtime.GOOS == "windows" {
		return strings.ToLower(cleaned)
	}
	return cleaned
}

func (s *serverFileSources) sourceList() *cxlist.List[serverFileSource] {
	if s == nil {
		return nil
	}
	return s.sources.Load()
}

func (s *serverFileSources) sourceCount() int {
	sources := s.sourceList()
	if sources == nil {
		return 0
	}
	return sources.Len()
}

func (s *serverFileSources) empty() bool {
	return s.sourceCount() == 0
}

func (s *serverFileSources) first() *source.LocalFS {
	sources := s.sourceList()
	if sources == nil {
		return nil
	}
	entry, ok := sources.GetFirst()
	if !ok {
		return nil
	}
	return entry.files
}
func newServerFileSource(src *source.LocalFS, fallbackRoot string, logger *slog.Logger) *source.LocalFS {
	if src != nil && src.Root() != "" {
		return src
	}
	return serverFileSourceFromRoot(fallbackRoot, logger)
}

func serverFileSourceFromRoot(fallbackRoot string, logger *slog.Logger) *source.LocalFS {
	files, ok, err := source.NewLocalDirectory(fallbackRoot)
	if err != nil {
		warnServerFileSource(logger, "Local file source unavailable", err)
		return nil
	}
	if !ok {
		return nil
	}
	return files
}

func serverFileSourceFromOptionalRoot(fallbackRoot string, logger *slog.Logger) *source.LocalFS {
	files, ok, err := source.NewLocalDirectory(fallbackRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		warnServerFileSource(logger, "Local file source unavailable", err)
		return nil
	}
	if !ok {
		return nil
	}
	return files
}

func warnServerFileSource(logger *slog.Logger, message string, err error) {
	if logger == nil {
		return
	}
	logger.Warn(message, slog.Any("error", err))
}

func (s *serverFileSources) ReadFile(path string) ([]byte, error) {
	file, _, err := s.OpenFile(path)
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil {
		return nil, oops.Wrapf(readErr, "read local asset file")
	}
	if closeErr != nil {
		return nil, oops.Wrapf(closeErr, "close local asset file")
	}
	return body, nil
}

func (s *serverFileSources) OpenFile(path string) (*os.File, fs.FileInfo, error) {
	if s.empty() {
		return nil, nil, oops.Owner("server").Wrap(fmt.Errorf("local file source is required for %s", path))
	}
	var file *os.File
	var info fs.FileInfo
	found := false
	sources := s.sourceList()
	errs := make([]error, 0, sources.Len())
	sources.Range(func(_ int, entry serverFileSource) bool {
		var err error
		file, info, err = entry.files.OpenFile(path)
		if err == nil {
			found = true
			return false
		}
		errs = append(errs, err)
		return true
	})
	if found {
		return file, info, nil
	}
	return nil, nil, oops.Owner("server").Wrap(fmt.Errorf("open local asset file: %w", errors.Join(errs...)))
}
