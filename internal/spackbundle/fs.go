package spackbundle

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

func cleanupTempBundle(path string, committed *bool) {
	if committed != nil && *committed {
		return
	}
	discardError(os.Remove(path))
}

func closeBundleWriters(zipWriter *zip.Writer, temp *os.File, err error) error {
	if closeErr := zipWriter.Close(); closeErr != nil {
		err = oops.Join(err, oops.Wrapf(closeErr, "close bundle zip writer"))
	}
	return closeBundleFile(temp, err)
}

func closeBundleFile(file *os.File, err error) error {
	if closeErr := file.Close(); closeErr != nil {
		return oops.In("spackbundle").Owner("fs").Wrap(oops.Join(err, oops.Wrapf(closeErr, "close bundle temp file")))
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
