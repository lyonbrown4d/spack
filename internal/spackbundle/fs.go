package spackbundle

import (
	"archive/zip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func cleanupTempBundle(path string, committed *bool) {
	if committed != nil && *committed {
		return
	}
	discardError(os.Remove(path))
}

func closeBundleWriters(zipWriter *zip.Writer, temp *os.File, err error) error {
	if closeErr := zipWriter.Close(); closeErr != nil {
		err = errors.Join(err, fmt.Errorf("close bundle zip writer: %w", closeErr))
	}
	return closeBundleFile(temp, err)
}

func closeBundleFile(file *os.File, err error) error {
	if closeErr := file.Close(); closeErr != nil {
		return errors.Join(err, fmt.Errorf("close bundle temp file: %w", closeErr))
	}
	return err
}

func discardError(err error) {
	_ = err
}

func isPathInside(root, fullPath string) bool {
	rel, err := filepath.Rel(root, fullPath)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
