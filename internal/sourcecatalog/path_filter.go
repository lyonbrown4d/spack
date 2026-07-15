package sourcecatalog

import (
	"path"
	"path/filepath"
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/samber/oops"
)

type assetPathFilter struct {
	include *cxlist.List[string]
	exclude *cxlist.List[string]
	err     error
}

func newAssetPathFilter(assets *config.Assets) assetPathFilter {
	if assets == nil {
		return assetPathFilter{}
	}

	include, includeErr := normalizeGlobPatterns("assets.include", assets.Include)
	if includeErr != nil {
		return assetPathFilter{err: includeErr}
	}
	exclude, excludeErr := normalizeGlobPatterns("assets.exclude", assets.Exclude)
	if excludeErr != nil {
		return assetPathFilter{err: excludeErr}
	}
	return assetPathFilter{
		include: include,
		exclude: exclude,
	}
}

func (f assetPathFilter) Err() error {
	return f.err
}

func (f assetPathFilter) Allow(rawPath string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}

	assetPath := normalizeSourcePath(rawPath)
	if assetPath == "" {
		return false, nil
	}
	if globPatternsEmpty(f.include) && globPatternsEmpty(f.exclude) {
		return true, nil
	}

	included, err := f.matchesInclude(assetPath)
	if err != nil || !included {
		return false, err
	}
	excluded, err := matchesAnyGlob(f.exclude, assetPath)
	if err != nil || excluded {
		return false, err
	}
	return true, nil
}

func (f assetPathFilter) matchesInclude(assetPath string) (bool, error) {
	if globPatternsEmpty(f.include) {
		return true, nil
	}
	return matchesAnyGlob(f.include, assetPath)
}

func matchesAnyGlob(patterns *cxlist.List[string], assetPath string) (bool, error) {
	if globPatternsEmpty(patterns) {
		return false, nil
	}

	var matched bool
	var matchErr error
	patterns.Range(func(_ int, pattern string) bool {
		ok, err := doublestar.Match(pattern, assetPath)
		if err != nil {
			matchErr = oops.Wrapf(err, "match asset path %q with glob %q", assetPath, pattern)
			return false
		}
		if ok {
			matched = true
			return false
		}
		return true
	})
	if matchErr != nil {
		return false, matchErr
	}
	return matched, nil
}

func normalizeGlobPatterns(field string, patterns []string) (*cxlist.List[string], error) {
	normalized := cxlist.NewListWithCapacity[string](len(patterns))
	for _, rawPattern := range patterns {
		pattern := normalizeGlobPattern(rawPattern)
		if pattern == "" {
			continue
		}
		if !doublestar.ValidatePattern(pattern) {
			return nil, oops.Errorf("invalid %s glob pattern %q", field, rawPattern)
		}
		normalized.Add(pattern)
	}
	return normalized, nil
}

func globPatternsEmpty(patterns *cxlist.List[string]) bool {
	return patterns == nil || patterns.IsEmpty()
}

func normalizeGlobPattern(rawPattern string) string {
	pattern := strings.TrimSpace(rawPattern)
	if pattern == "" {
		return ""
	}
	pattern = filepath.ToSlash(pattern)
	pattern = strings.TrimPrefix(pattern, "/")
	pattern = path.Clean(pattern)
	if pattern == "." {
		return ""
	}
	return pattern
}
