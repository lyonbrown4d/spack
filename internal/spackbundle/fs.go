package spackbundle

import (
	"archive/tar"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/samber/oops"
)

func cleanupTempBundle(path string, committed *bool) {
	if committed != nil && *committed {
		return
	}
	discardBestEffortCleanupError(os.Remove(path))
}

func closeBundleWriters(tarWriter *tar.Writer, zstdWriter *zstd.Encoder, temp *os.File, err error) error {
	if closeErr := tarWriter.Close(); closeErr != nil {
		err = oops.Join(err, oops.Wrapf(closeErr, "close bundle tar writer"))
	}
	return closeBundleZstdWriter(zstdWriter, temp, err)
}

func closeBundleZstdWriter(zstdWriter *zstd.Encoder, temp *os.File, err error) error {
	if closeErr := zstdWriter.Close(); closeErr != nil {
		err = oops.Join(err, oops.Wrapf(closeErr, "close bundle zstd writer"))
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

// discardBestEffortCleanupError intentionally ignores cleanup failures on paths
// that are only used as temporary build artifacts. Callers returning a primary
// error should join cleanup errors instead of using this helper.
func discardBestEffortCleanupError(err error) {
	_ = err
}

func isPathInside(root, fullPath string) bool {
	rel, err := filepath.Rel(root, fullPath)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
