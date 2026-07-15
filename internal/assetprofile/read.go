package assetprofile

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/arcgolabs/collectionx/bytex"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/oops"
)

type bundleAssetReader struct {
	source io.ReadCloser
	bundle *spackbundle.Reader
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
		return 0, oops.Wrapf(err, "open asset profile file")
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
		return read, oops.Wrapf(closeErr, "close bundle asset for profile")
	}
	return read, nil
}

func openBundleReferenceAsset(reference string) (*bundleAssetReader, uint64, error) {
	ref, err := spackbundle.ParseReference(reference)
	if err != nil {
		return nil, 0, oops.Wrapf(err, "parse bundle asset reference")
	}
	reader, err := spackbundle.OpenReader(ref.BundlePath)
	if err != nil {
		return nil, 0, oops.Wrapf(err, "open bundle for asset profile")
	}
	stat, err := reader.Stat(ref.FilePath)
	if err != nil {
		discardClose(reader)
		return nil, 0, oops.Wrapf(err, "stat bundle asset for profile")
	}
	source, err := reader.OpenFile(ref.FilePath)
	if err != nil {
		discardClose(reader)
		return nil, 0, oops.Wrapf(err, "open bundle asset for profile")
	}
	return &bundleAssetReader{source: source, bundle: reader}, stat.Size, nil
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
		return nil, oops.Wrapf(err, "resolve asset profile file")
	}
	root, err := os.OpenRoot(filepath.Dir(cleaned))
	if err != nil {
		return nil, oops.Wrapf(err, "open asset profile root")
	}
	file, openErr := root.Open(filepath.Base(cleaned))
	closeErr := root.Close()
	if openErr != nil {
		return nil, oops.Wrapf(openErr, "open asset profile file in root")
	}
	if closeErr != nil {
		discardClose(file)
		return nil, oops.Wrapf(closeErr, "close asset profile root")
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
	const maxInt64Uint = uint64(1<<63 - 1)
	if size > maxInt64Uint {
		return limit
	}
	return int64(size)
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
		return total, oops.Wrapf(err, "read asset profile bytes")
	}
}

func closeReader(closer io.Closer) error {
	if closer == nil {
		return nil
	}
	if err := closer.Close(); err != nil {
		return oops.Wrapf(err, "close asset profile reader")
	}
	return nil
}

func discardClose(closer io.Closer) {
	if err := closeReader(closer); err != nil {
		return
	}
}
