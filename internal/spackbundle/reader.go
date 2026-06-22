package spackbundle

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Reader is a closeable handle for reading files from one SPACK bundle.
type Reader struct {
	path        string
	archive     *zip.ReadCloser
	files       map[string]*zip.File
	index       Index
	indexLoaded bool
}

// FileStat describes one file embedded in a SPACK bundle.
type FileStat struct {
	Path string
	Size uint64
}

// OpenReader opens a SPACK bundle once and indexes its zip entries by bundle path.
func OpenReader(bundlePath string) (*Reader, error) {
	absolute, err := normalizedBundlePath(bundlePath)
	if err != nil {
		return nil, err
	}
	archive, err := zip.OpenReader(absolute)
	if err != nil {
		return nil, fmt.Errorf("open bundle: %w", err)
	}
	return &Reader{
		path:    absolute,
		archive: archive,
		files:   indexZipFiles(archive.File),
	}, nil
}

// Path returns the normalized absolute bundle path.
func (r *Reader) Path() string {
	if r == nil {
		return ""
	}
	return r.path
}

// Close closes the underlying bundle archive.
func (r *Reader) Close() error {
	if r == nil || r.archive == nil {
		return nil
	}
	if err := r.archive.Close(); err != nil {
		return fmt.Errorf("close bundle: %w", err)
	}
	r.archive = nil
	return nil
}

// Index reads and validates the embedded bundle index. The decoded index is cached on the reader.
func (r *Reader) Index() (Index, error) {
	if r == nil {
		return Index{}, errors.New("bundle reader is nil")
	}
	if r.indexLoaded {
		return r.index, nil
	}
	file, ok := r.files[IndexPath]
	if !ok {
		return Index{}, fmt.Errorf("bundle index %q was not found", IndexPath)
	}
	body, err := readZipFile(file, IndexPath)
	if err != nil {
		return Index{}, err
	}
	index, err := unmarshalIndex(body)
	if err != nil {
		return Index{}, err
	}
	r.index = index
	r.indexLoaded = true
	return index, nil
}

// ReadFile reads one file from the bundle.
func (r *Reader) ReadFile(filePath string) ([]byte, error) {
	file, cleanPath, err := r.lookupFile(filePath)
	if err != nil {
		return nil, err
	}
	return readZipFile(file, cleanPath)
}

// OpenFile opens one file from the bundle. The caller must close the returned reader.
func (r *Reader) OpenFile(filePath string) (io.ReadCloser, error) {
	file, cleanPath, err := r.lookupFile(filePath)
	if err != nil {
		return nil, err
	}
	source, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("open bundle file %q: %w", cleanPath, err)
	}
	return source, nil
}

// Stat returns size metadata for one bundle file.
func (r *Reader) Stat(filePath string) (FileStat, error) {
	file, cleanPath, err := r.lookupFile(filePath)
	if err != nil {
		return FileStat{}, err
	}
	return FileStat{
		Path: cleanPath,
		Size: file.UncompressedSize64,
	}, nil
}

func (r *Reader) lookupFile(filePath string) (*zip.File, string, error) {
	if r == nil {
		return nil, "", errors.New("bundle reader is nil")
	}
	cleanPath, err := cleanBundlePath(filePath)
	if err != nil {
		return nil, "", err
	}
	file, ok := r.files[cleanPath]
	if !ok {
		return nil, "", os.ErrNotExist
	}
	return file, cleanPath, nil
}

func indexZipFiles(files []*zip.File) map[string]*zip.File {
	indexed := make(map[string]*zip.File, len(files))
	for _, file := range files {
		if file == nil {
			continue
		}
		name := filepath.ToSlash(file.Name)
		if _, exists := indexed[name]; !exists {
			indexed[name] = file
		}
	}
	return indexed
}
