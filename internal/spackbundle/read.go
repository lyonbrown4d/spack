package spackbundle

import (
	"path/filepath"
	"strings"
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
		return Index{}, err
	}
	defer func() {
		discardError(reader.Close())
	}()
	return reader.Index()
}

// ReadFile reads one file from a SPACK bundle.
func ReadFile(bundlePath, filePath string) ([]byte, error) {
	reader, err := OpenReader(bundlePath)
	if err != nil {
		return nil, err
	}
	defer func() {
		discardError(reader.Close())
	}()
	return reader.ReadFile(filePath)
}
