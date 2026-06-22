package sourcecatalog

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/lyonbrown4d/spack/internal/config"
)

type assetPathFilter struct {
	include []string
	exclude []string
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
	if len(f.include) == 0 && len(f.exclude) == 0 {
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
	if len(f.include) == 0 {
		return true, nil
	}
	return matchesAnyGlob(f.include, assetPath)
}

func matchesAnyGlob(patterns []string, assetPath string) (bool, error) {
	for _, pattern := range patterns {
		matched, err := doublestar.Match(pattern, assetPath)
		if err != nil {
			return false, fmt.Errorf("match asset path %q with glob %q: %w", assetPath, pattern, err)
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

func normalizeGlobPatterns(field string, patterns []string) ([]string, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	normalized := make([]string, 0, len(patterns))
	for _, rawPattern := range patterns {
		pattern := normalizeGlobPattern(rawPattern)
		if pattern == "" {
			continue
		}
		if !doublestar.ValidatePattern(pattern) {
			return nil, fmt.Errorf("invalid %s glob pattern %q", field, rawPattern)
		}
		normalized = append(normalized, pattern)
	}
	return normalized, nil
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
