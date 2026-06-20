package spackbundle

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

func cleanBundlePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if err := validateRawBundlePath(raw, trimmed); err != nil {
		return "", err
	}
	cleaned := path.Clean(filepath.ToSlash(trimmed))
	if err := validateCleanBundlePath(raw, cleaned); err != nil {
		return "", err
	}
	return cleaned, nil
}

func validateRawBundlePath(raw, trimmed string) error {
	if trimmed == "" {
		return errors.New("bundle path is empty")
	}
	if strings.ContainsRune(trimmed, '\x00') || strings.ContainsRune(trimmed, '\\') {
		return fmt.Errorf("bundle path %q contains an invalid character", raw)
	}
	if filepath.IsAbs(trimmed) || path.IsAbs(trimmed) {
		return fmt.Errorf("bundle path %q must be relative", raw)
	}
	return nil
}

func validateCleanBundlePath(raw, cleaned string) error {
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("bundle path %q escapes the bundle root", raw)
	}
	if cleaned == ".spack" || strings.HasPrefix(cleaned, ".spack/") {
		return fmt.Errorf("bundle path %q uses reserved .spack metadata namespace", raw)
	}
	return nil
}

func isMetadataPath(name string) bool {
	cleaned := path.Clean(filepath.ToSlash(name))
	return cleaned == ".spack" || strings.HasPrefix(cleaned, ".spack/")
}
