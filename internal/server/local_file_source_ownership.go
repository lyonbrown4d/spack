package server

import (
	"errors"
	"fmt"
	"io/fs"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/samber/oops"
)

func newServerFileSourcesWithEntries(entries ...serverFileSource) *serverFileSources {
	sources := &serverFileSources{
		trustedRoots: make(map[*source.LocalFS]fs.FS),
	}
	sources.sources.Store(cxlist.NewList(entries...))
	return sources
}
func mergeServerFileSources(groups ...*serverFileSources) *serverFileSources {
	merged := newServerFileSourcesWithEntries()
	mergedSources := cxlist.NewListWithCapacity[serverFileSource](len(groups))
	seen := make(map[string]*source.LocalFS, len(groups))
	for _, group := range groups {
		mergeServerFileSourceGroup(merged, mergedSources, seen, group)
	}
	if mergedSources.IsEmpty() {
		return nil
	}
	merged.sources.Store(mergedSources)
	return merged
}

func mergeServerFileSourceGroup(
	merged *serverFileSources,
	mergedSources *cxlist.List[serverFileSource],
	seen map[string]*source.LocalFS,
	group *serverFileSources,
) {
	if group == nil {
		return
	}
	merged.recordCleanupError(group.cleanupError())
	groupSources := group.sourceList()
	if groupSources == nil {
		return
	}
	groupSources.Range(func(_ int, entry serverFileSource) bool {
		mergeServerFileSource(merged, mergedSources, seen, entry)
		return true
	})
}

func mergeServerFileSource(
	merged *serverFileSources,
	mergedSources *cxlist.List[serverFileSource],
	seen map[string]*source.LocalFS,
	entry serverFileSource,
) {
	if entry.files == nil || entry.files.Root() == "" {
		merged.discardOwned(entry)
		return
	}
	key := serverFileSourceKey(entry.files.Root())
	if kept, ok := seen[key]; ok {
		if kept != entry.files {
			merged.discardOwned(entry)
		}
		return
	}
	seen[key] = entry.files
	mergedSources.Add(entry)
}

func (s *serverFileSources) discardOwned(entry serverFileSource) {
	if !entry.owned || entry.files == nil {
		return
	}
	if err := entry.files.Cleanup(); err != nil {
		s.recordCleanupError(oops.Wrapf(err, "cleanup duplicate local file source"))
	}
}

func (s *serverFileSources) recordCleanupError(err error) {
	if s == nil || err == nil {
		return
	}
	s.cleanupErrMu.Lock()
	s.cleanupErr = errors.Join(s.cleanupErr, err)
	s.cleanupErrMu.Unlock()
}

func (s *serverFileSources) cleanupError() error {
	if s == nil {
		return nil
	}
	s.cleanupErrMu.Lock()
	defer s.cleanupErrMu.Unlock()
	return s.cleanupErr
}

func (s *serverFileSources) TrustedReadOnlyPath(fullPath string) (fs.FS, string, bool, error) {
	if s.empty() {
		return nil, "", false, nil
	}

	result, errs := s.searchTrustedReadOnlyPath(fullPath)
	if result.trusted {
		return result.rootFS, result.relativePath, true, nil
	}
	if len(errs) == 0 {
		return nil, "", false, nil
	}
	return nil, "", false, oops.Owner("server").Wrap(
		fmt.Errorf("resolve trusted local asset path: %w", errors.Join(errs...)),
	)
}

func (s *serverFileSources) searchTrustedReadOnlyPath(
	fullPath string,
) (trustedReadOnlyPathResult, []error) {
	sources := s.sourceList()
	errs := make([]error, 0, sources.Len())
	var found trustedReadOnlyPathResult
	sources.Range(func(_ int, entry serverFileSource) bool {
		result := s.trustedReadOnlyPath(entry, fullPath)
		if result.err != nil {
			errs = append(errs, result.err)
			return true
		}
		if !result.trusted {
			return true
		}
		found = result
		return false
	})
	return found, errs
}

type trustedReadOnlyPathResult struct {
	rootFS       fs.FS
	relativePath string
	trusted      bool
	err          error
}

func (s *serverFileSources) trustedReadOnlyPath(
	entry serverFileSource,
	fullPath string,
) trustedReadOnlyPathResult {
	rootFS, relativePath, trusted, err := entry.files.TrustedReadOnlyPath(fullPath)
	if err != nil || !trusted {
		return trustedReadOnlyPathResult{
			rootFS:       rootFS,
			relativePath: relativePath,
			trusted:      trusted,
			err:          err,
		}
	}
	return trustedReadOnlyPathResult{
		rootFS:       s.cachedTrustedRoot(entry.files, rootFS),
		relativePath: relativePath,
		trusted:      true,
	}
}

func (s *serverFileSources) cachedTrustedRoot(files *source.LocalFS, rootFS fs.FS) fs.FS {
	s.trustedRootsMu.Lock()
	defer s.trustedRootsMu.Unlock()

	if s.trustedRoots == nil {
		s.trustedRoots = make(map[*source.LocalFS]fs.FS)
	}
	if cached := s.trustedRoots[files]; cached != nil {
		return cached
	}
	s.trustedRoots[files] = rootFS
	return rootFS
}

func (s *serverFileSources) Cleanup() error {
	if s == nil {
		return nil
	}
	s.cleanupOnce.Do(func() {
		s.sourcesMu.Lock()
		s.closed = true
		sources := s.sources.Load()
		s.sourcesMu.Unlock()
		s.cleanupOwnedSources(sources)
	})
	if err := s.cleanupError(); err != nil {
		return oops.Owner("server").Wrap(err)
	}
	return nil
}

func (s *serverFileSources) cleanupOwnedSources(sources *cxlist.List[serverFileSource]) {
	cleaned := make(map[*source.LocalFS]struct{}, s.sourceCount())
	if sources != nil {
		sources.Range(func(_ int, entry serverFileSource) bool {
			s.cleanupOwnedSource(cleaned, entry)
			return true
		})
	}
	s.trustedRootsMu.Lock()
	s.trustedRoots = nil
	s.trustedRootsMu.Unlock()
}

func (s *serverFileSources) cleanupOwnedSource(
	cleaned map[*source.LocalFS]struct{},
	entry serverFileSource,
) {
	if !entry.owned || entry.files == nil {
		return
	}
	if _, ok := cleaned[entry.files]; ok {
		return
	}
	cleaned[entry.files] = struct{}{}
	if err := entry.files.Cleanup(); err != nil {
		s.recordCleanupError(oops.Wrapf(err, "cleanup local file source"))
	}
}
