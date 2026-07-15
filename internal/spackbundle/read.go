package spackbundle

import (
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

const maxExtractedFileBytes = 2 << 30

// Extracted describes an extracted SPACK bundle.
type Extracted struct {
	BundlePath  string
	Root        string
	Index       Index
	cleanupRoot string
}

// IsBundlePath reports whether bundlePath names a SPACK bundle candidate.
func IsBundlePath(bundlePath string) bool {
	return strings.EqualFold(filepath.Ext(strings.TrimSpace(bundlePath)), ".spack")
}

// ReadIndex reads and validates the index embedded in a SPACK bundle.
func ReadIndex(bundlePath string) (Index, error) {
	reader, err := OpenReader(bundlePath)
	if err != nil {
		return Index{}, oops.In("spackbundle").Owner("read index").With("bundle_path", bundlePath).Wrap(err)
	}
	defer func() {
		discardError(reader.Close())
	}()
	index, err := reader.Index()
	if err != nil {
		return Index{}, oops.In("spackbundle").Owner("read index").With("bundle_path", bundlePath).Wrap(err)
	}
	return index, nil
}

// ReadFile reads one file from a SPACK bundle.
func ReadFile(bundlePath, filePath string) ([]byte, error) {
	reader, err := OpenReader(bundlePath)
	if err != nil {
		return nil, oops.In("spackbundle").Owner("read file").With("bundle_path", bundlePath).With("file_path", filePath).Wrap(err)
	}
	defer func() {
		discardError(reader.Close())
	}()
	body, err := reader.ReadFile(filePath)
	if err != nil {
		return nil, oops.In("spackbundle").Owner("read file").With("bundle_path", bundlePath).With("file_path", filePath).Wrap(err)
	}
	return body, nil
}
