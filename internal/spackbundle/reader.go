package spackbundle

import (
	"bytes"
	"errors"
	"io"
	"os"

	"github.com/samber/oops"
)

// Reader is a closeable handle for reading files from one SPACK bundle.
type Reader struct {
	path        string
	index       Index
	indexLoaded bool
}

// FileStat describes one file embedded in a SPACK bundle.
type FileStat struct {
	Path string
	Size uint64
}

// OpenReader opens a SPACK bundle handle.
func OpenReader(bundlePath string) (*Reader, error) {
	absolute, err := normalizedBundlePath(bundlePath)
	if err != nil {
		return nil, err
	}
	if err := checkBundleMagic(absolute); err != nil {
		return nil, err
	}
	return &Reader{path: absolute}, nil
}

// Path returns the normalized absolute bundle path.
func (r *Reader) Path() string {
	if r == nil {
		return ""
	}
	return r.path
}

// Close closes the bundle handle.
func (r *Reader) Close() error {
	return nil
}

// Index reads and validates the embedded bundle index. The decoded index is cached on the reader.
func (r *Reader) Index() (Index, error) {
	if r == nil {
		return Index{}, oops.In("spackbundle").Owner("reader").Wrap(errors.New("bundle reader is nil"))
	}
	if r.indexLoaded {
		return r.index, nil
	}
	body, err := readBundleEntry(r.path, IndexPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Index{}, oops.Errorf("bundle index %q was not found", IndexPath)
		}
		return Index{}, err
	}
	index, err := unmarshalIndex(body)
	if err != nil {
		return Index{}, err
	}
	if err := validateIndex(index); err != nil {
		return Index{}, err
	}
	r.index = index
	r.indexLoaded = true
	return index, nil
}

// ReadFile reads one file from the bundle.
func (r *Reader) ReadFile(filePath string) ([]byte, error) {
	cleanPath, expected, err := r.lookupIndexFile(filePath)
	if err != nil {
		return nil, err
	}
	body, err := readBundleEntry(r.path, cleanPath)
	if err != nil {
		return nil, err
	}
	if err := verifyBody(expected, body); err != nil {
		return nil, err
	}
	return body, nil
}

// OpenFile opens one file from the bundle. The caller must close the returned reader.
func (r *Reader) OpenFile(filePath string) (io.ReadCloser, error) {
	body, err := r.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

// Stat returns size metadata for one bundle file.
func (r *Reader) Stat(filePath string) (FileStat, error) {
	cleanPath, file, err := r.lookupIndexFile(filePath)
	if err != nil {
		return FileStat{}, err
	}
	if file.Size < 0 {
		return FileStat{}, oops.Errorf("bundle file %q has negative size", cleanPath)
	}
	return FileStat{Path: cleanPath, Size: uint64(file.Size)}, nil
}

func (r *Reader) lookupIndexFile(filePath string) (string, IndexFile, error) {
	if r == nil {
		return "", IndexFile{}, oops.In("spackbundle").Owner("reader").Wrap(errors.New("bundle reader is nil"))
	}
	cleanPath, err := cleanBundlePath(filePath)
	if err != nil {
		return "", IndexFile{}, err
	}
	index, err := r.Index()
	if err != nil {
		return "", IndexFile{}, err
	}
	file, ok := indexFileMap(index)[cleanPath]
	if !ok {
		return "", IndexFile{}, os.ErrNotExist
	}
	return cleanPath, file, nil
}

func readBundleEntry(bundlePath, filePath string) ([]byte, error) {
	stream, err := openBundleStream(bundlePath)
	if err != nil {
		return nil, err
	}
	defer func() {
		discardError(stream.Close())
	}()

	for {
		header, err := stream.tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil, os.ErrNotExist
		}
		if err != nil {
			return nil, oops.Wrapf(err, "read bundle tar entry")
		}
		path, err := cleanTarEntryPath(header)
		if err != nil {
			return nil, err
		}
		if path != filePath {
			continue
		}
		return readTarEntryBody(stream.tarReader, header, path)
	}
}
