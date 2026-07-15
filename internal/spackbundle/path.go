package spackbundle

import (
	"errors"
	"path"
	"path/filepath"
	"strings"

	"github.com/lyonbrown4d/spack/pkg"
	"github.com/samber/oops"
)

func cleanBundlePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if err := validateRawBundlePath(raw, trimmed); err != nil {
		return "", oops.In("spackbundle").Owner("path").Wrap(err)
	}
	cleaned := path.Clean(filepath.ToSlash(trimmed))
	if err := validateCleanBundlePath(raw, cleaned); err != nil {
		return "", oops.In("spackbundle").Owner("path").Wrap(err)
	}
	return cleaned, nil
}

func validateRawBundlePath(raw, trimmed string) error {
	if trimmed == "" {
		return errors.New("bundle path is empty")
	}
	if strings.ContainsRune(trimmed, '\x00') || strings.ContainsRune(trimmed, '\\') {
		return oops.Errorf("bundle path %q contains an invalid character", raw)
	}
	if filepath.IsAbs(trimmed) || path.IsAbs(trimmed) {
		return oops.Errorf("bundle path %q must be relative", raw)
	}
	return nil
}

func validateCleanBundlePath(raw, cleaned string) error {
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return oops.Errorf("bundle path %q escapes the bundle root", raw)
	}
	if pkg.HasUnsafePortablePathSegment(cleaned) {
		return oops.Errorf("bundle path %q contains an unsafe portable path segment", raw)
	}
	if cleaned == ".spack" || strings.HasPrefix(cleaned, ".spack/") {
		return oops.Errorf("bundle path %q uses reserved .spack metadata namespace", raw)
	}
	return nil
}

func isMetadataPath(name string) bool {
	cleaned := path.Clean(filepath.ToSlash(name))
	return cleaned == ".spack" || strings.HasPrefix(cleaned, ".spack/")
}
