package spackbundle

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxExtractedFileBytes = 2 << 30

// Extracted describes an extracted SPACK bundle.
type Extracted struct {
	BundlePath string
	Root       string
	Index      Index
}

// IsBundlePath reports whether path names a SPACK bundle candidate.
func IsBundlePath(path string) bool {
	return strings.EqualFold(filepath.Ext(strings.TrimSpace(path)), ".spack")
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

// Extract unpacks a SPACK bundle into a temporary directory.
func Extract(ctx context.Context, bundlePath string) (Extracted, error) {
	reader, err := OpenReader(bundlePath)
	if err != nil {
		return Extracted{}, err
	}
	defer func() {
		discardError(reader.Close())
	}()

	index, err := reader.Index()
	if err != nil {
		return Extracted{}, err
	}

	root, err := os.MkdirTemp("", "spack-bundle-*")
	if err != nil {
		return Extracted{}, fmt.Errorf("create bundle extraction directory: %w", err)
	}
	committed := false
	defer cleanupExtractedRoot(root, &committed)

	if err := extractFiles(ctx, root, reader.archive.File); err != nil {
		return Extracted{}, err
	}
	committed = true
	return Extracted{
		BundlePath: reader.Path(),
		Root:       root,
		Index:      index,
	}, nil
}

// Cleanup removes extracted bundle contents.
func (e Extracted) Cleanup() error {
	if strings.TrimSpace(e.Root) == "" {
		return nil
	}
	if err := os.RemoveAll(e.Root); err != nil {
		return fmt.Errorf("cleanup extracted bundle root: %w", err)
	}
	return nil
}

func extractFiles(ctx context.Context, root string, files []*zip.File) error {
	for _, file := range files {
		if file == nil || file.FileInfo().IsDir() || isMetadataPath(file.Name) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("extract bundle canceled: %w", err)
		}
		path, err := cleanBundlePath(file.Name)
		if err != nil {
			return err
		}
		if err := extractFile(root, path, file); err != nil {
			return err
		}
	}
	return nil
}

func extractFile(root, path string, file *zip.File) error {
	target := filepath.Join(root, filepath.FromSlash(path))
	if !isPathInside(root, target) {
		return fmt.Errorf("bundle file %q escapes extraction root", path)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return fmt.Errorf("create bundle file directory %q: %w", path, err)
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return fmt.Errorf("open extraction root: %w", err)
	}
	defer func() {
		discardError(rootHandle.Close())
	}()

	source, err := file.Open()
	if err != nil {
		return fmt.Errorf("open bundle file %q: %w", path, err)
	}
	defer func() {
		discardError(source.Close())
	}()

	targetFile, err := rootHandle.OpenFile(filepath.FromSlash(path), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create extracted bundle file %q: %w", path, err)
	}
	defer func() {
		discardError(targetFile.Close())
	}()
	written, err := copyZipFilePayload(targetFile, source, file.UncompressedSize64)
	if err != nil {
		return fmt.Errorf("extract bundle file %q: %w", path, err)
	}
	if written < 0 || uint64(written) != file.UncompressedSize64 {
		return fmt.Errorf("bundle file %q size mismatch", path)
	}
	return nil
}

func normalizedBundlePath(bundlePath string) (string, error) {
	bundlePath = strings.TrimSpace(bundlePath)
	if bundlePath == "" {
		return "", errors.New("bundle path is required")
	}
	absolute, err := filepath.Abs(filepath.Clean(bundlePath))
	if err != nil {
		return "", fmt.Errorf("resolve bundle path: %w", err)
	}
	return absolute, nil
}

func readZipFile(file *zip.File, path string) ([]byte, error) {
	if file.UncompressedSize64 > maxExtractedFileBytes {
		return nil, fmt.Errorf("bundle file %q exceeds max extracted bytes", path)
	}
	source, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("open bundle file %q: %w", path, err)
	}
	defer func() {
		discardError(source.Close())
	}()
	reader := io.LimitReader(source, int64(file.UncompressedSize64)+1)
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read bundle file %q: %w", path, err)
	}
	if uint64(len(body)) != file.UncompressedSize64 {
		return nil, fmt.Errorf("bundle file %q size mismatch", path)
	}
	return body, nil
}

func copyZipFilePayload(target io.Writer, source io.Reader, size uint64) (int64, error) {
	if size > maxExtractedFileBytes {
		return 0, errors.New("bundle file exceeds max extracted bytes")
	}
	reader := io.LimitReader(source, int64(size)+1)
	written, err := io.Copy(target, reader)
	if err != nil {
		return 0, fmt.Errorf("copy bundle file payload: %w", err)
	}
	return written, nil
}

func cleanupExtractedRoot(root string, committed *bool) {
	if committed != nil && *committed {
		return
	}
	discardError(os.RemoveAll(root))
}
