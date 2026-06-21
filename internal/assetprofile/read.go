package assetprofile

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/arcgolabs/collectionx/bytex"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

type bundleAssetReader struct {
	source io.ReadCloser
	bundle *zip.ReadCloser
}

func countAssetBytes(fullPath string, limit int64, counter *bytex.Counter) (int64, error) {
	fullPath = strings.TrimSpace(fullPath)
	if fullPath == "" || limit <= 0 {
		return 0, nil
	}
	if spackbundle.IsReference(fullPath) {
		return countBundleReferenceBytes(fullPath, limit, counter)
	}
	file, err := openAssetFile(fullPath)
	if err != nil {
		return 0, fmt.Errorf("open asset profile file: %w", err)
	}
	defer discardClose(file)
	return countReaderBytes(file, limit, counter)
}

func countBundleReferenceBytes(reference string, limit int64, counter *bytex.Counter) (int64, error) {
	reader, size, err := openBundleReferenceAsset(reference)
	if err != nil {
		return 0, err
	}
	read, readErr := countReaderBytes(reader.source, minPositiveUint64(limit, size), counter)
	closeErr := reader.Close()
	if readErr != nil {
		return read, readErr
	}
	if closeErr != nil {
		return read, fmt.Errorf("close bundle asset for profile: %w", closeErr)
	}
	return read, nil
}

func openBundleReferenceAsset(reference string) (*bundleAssetReader, uint64, error) {
	ref, err := spackbundle.ParseReference(reference)
	if err != nil {
		return nil, 0, fmt.Errorf("parse bundle asset reference: %w", err)
	}
	reader, err := zip.OpenReader(ref.BundlePath)
	if err != nil {
		return nil, 0, fmt.Errorf("open bundle for asset profile: %w", err)
	}
	for _, file := range reader.File {
		if file == nil || filepath.ToSlash(file.Name) != ref.FilePath {
			continue
		}
		source, err := file.Open()
		if err != nil {
			discardClose(reader)
			return nil, 0, fmt.Errorf("open bundle asset for profile: %w", err)
		}
		return &bundleAssetReader{source: source, bundle: reader}, file.UncompressedSize64, nil
	}
	discardClose(reader)
	return nil, 0, os.ErrNotExist
}

func (r *bundleAssetReader) Close() error {
	if r == nil {
		return nil
	}
	sourceErr := closeReader(r.source)
	bundleErr := closeReader(r.bundle)
	if sourceErr != nil {
		return sourceErr
	}
	return bundleErr
}

func openAssetFile(fullPath string) (*os.File, error) {
	cleaned, err := filepath.Abs(filepath.Clean(fullPath))
	if err != nil {
		return nil, fmt.Errorf("resolve asset profile file: %w", err)
	}
	root, err := os.OpenRoot(filepath.Dir(cleaned))
	if err != nil {
		return nil, fmt.Errorf("open asset profile root: %w", err)
	}
	file, openErr := root.Open(filepath.Base(cleaned))
	closeErr := root.Close()
	if openErr != nil {
		return nil, fmt.Errorf("open asset profile file in root: %w", openErr)
	}
	if closeErr != nil {
		discardClose(file)
		return nil, fmt.Errorf("close asset profile root: %w", closeErr)
	}
	return file, nil
}

func minPositiveUint64(limit int64, size uint64) int64 {
	if limit <= 0 {
		return 0
	}
	if size >= uint64(limit) {
		return limit
	}
	parsed, err := strconv.ParseInt(strconv.FormatUint(size, 10), 10, 64)
	if err != nil {
		return limit
	}
	return parsed
}

func countReaderBytes(reader io.Reader, limit int64, counter *bytex.Counter) (int64, error) {
	limited := io.LimitReader(reader, limit)
	buffer := make([]byte, readBufferSize)
	var total int64
	for {
		n, err := limited.Read(buffer)
		if n > 0 {
			counter.Add(buffer[:n]...)
			total += int64(n)
		}
		if err == nil {
			continue
		}
		if err == io.EOF {
			return total, nil
		}
		return total, fmt.Errorf("read asset profile bytes: %w", err)
	}
}

func closeReader(closer io.Closer) error {
	if closer == nil {
		return nil
	}
	if err := closer.Close(); err != nil {
		return fmt.Errorf("close asset profile reader: %w", err)
	}
	return nil
}

func discardClose(closer io.Closer) {
	if err := closeReader(closer); err != nil {
		return
	}
}
