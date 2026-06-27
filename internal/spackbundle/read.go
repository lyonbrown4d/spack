package spackbundle

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
	"golang.org/x/sync/errgroup"
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

// ExtractReadOnly unpacks a SPACK bundle into a temporary read-only directory.
func ExtractReadOnly(ctx context.Context, bundlePath string) (Extracted, error) {
	extracted, err := Extract(ctx, bundlePath)
	if err != nil {
		return Extracted{}, err
	}
	if err := makeExtractedTreeReadOnly(extracted.Root); err != nil {
		discardError(extracted.Cleanup())
		return Extracted{}, err
	}
	return extracted, nil
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
		return Extracted{}, oops.Wrapf(err, "create bundle extraction directory")
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
	if err := makeExtractedTreeWritable(e.Root); err != nil && !errors.Is(err, os.ErrNotExist) {
		return oops.Wrapf(err, "prepare extracted bundle root cleanup")
	}
	if err := os.RemoveAll(e.Root); err != nil {
		return oops.Wrapf(err, "cleanup extracted bundle root")
	}
	return nil
}

type extractTask struct {
	path string
	file *zip.File
}

func extractFiles(ctx context.Context, root string, files []*zip.File) error {
	tasks, err := collectExtractTasks(files)
	if err != nil {
		return err
	}
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(bundleFileParallelism(len(tasks)))
	for index := range tasks {
		task := tasks[index]
		group.Go(func() error {
			if err := groupCtx.Err(); err != nil {
				return oops.Wrapf(err, "extract bundle canceled")
			}
			return extractFile(root, task.path, task.file)
		})
	}
	if err := group.Wait(); err != nil {
		return oops.Wrapf(err, "extract bundle files")
	}
	return nil
}

func collectExtractTasks(files []*zip.File) ([]extractTask, error) {
	tasks := make([]extractTask, 0, len(files))
	for _, file := range files {
		if file == nil || file.FileInfo().IsDir() || isMetadataPath(file.Name) {
			continue
		}
		path, err := cleanBundlePath(file.Name)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, extractTask{path: path, file: file})
	}
	return tasks, nil
}

func extractFile(root, path string, file *zip.File) error {
	target := filepath.Join(root, filepath.FromSlash(path))
	if !isPathInside(root, target) {
		return oops.Errorf("bundle file %q escapes extraction root", path)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return oops.Wrapf(err, "create bundle file directory %q", path)
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return oops.Wrapf(err, "open extraction root")
	}
	defer func() {
		discardError(rootHandle.Close())
	}()

	source, err := file.Open()
	if err != nil {
		return oops.Wrapf(err, "open bundle file %q", path)
	}
	defer func() {
		discardError(source.Close())
	}()

	targetFile, err := rootHandle.OpenFile(filepath.FromSlash(path), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return oops.Wrapf(err, "create extracted bundle file %q", path)
	}
	defer func() {
		discardError(targetFile.Close())
	}()
	written, err := copyZipFilePayload(targetFile, source, file.UncompressedSize64)
	if err != nil {
		return oops.Wrapf(err, "extract bundle file %q", path)
	}
	if written < 0 || uint64(written) != file.UncompressedSize64 {
		return oops.Errorf("bundle file %q size mismatch", path)
	}
	return nil
}

func normalizedBundlePath(bundlePath string) (string, error) {
	bundlePath = strings.TrimSpace(bundlePath)
	if bundlePath == "" {
		return "", oops.In("spackbundle").Owner("read").Wrap(errors.New("bundle path is required"))
	}
	absolute, err := filepath.Abs(filepath.Clean(bundlePath))
	if err != nil {
		return "", oops.Wrapf(err, "resolve bundle path")
	}
	return absolute, nil
}

func readZipFile(file *zip.File, path string) ([]byte, error) {
	if file.UncompressedSize64 > maxExtractedFileBytes {
		return nil, oops.Errorf("bundle file %q exceeds max extracted bytes", path)
	}
	source, err := file.Open()
	if err != nil {
		return nil, oops.Wrapf(err, "open bundle file %q", path)
	}
	defer func() {
		discardError(source.Close())
	}()
	reader := io.LimitReader(source, int64(file.UncompressedSize64)+1)
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, oops.Wrapf(err, "read bundle file %q", path)
	}
	if uint64(len(body)) != file.UncompressedSize64 {
		return nil, oops.Errorf("bundle file %q size mismatch", path)
	}
	return body, nil
}

func copyZipFilePayload(target io.Writer, source io.Reader, size uint64) (int64, error) {
	if size > maxExtractedFileBytes {
		return 0, oops.In("spackbundle").Owner("read").Wrap(errors.New("bundle file exceeds max extracted bytes"))
	}
	reader := io.LimitReader(source, int64(size)+1)
	written, err := io.Copy(target, reader)
	if err != nil {
		return 0, oops.Wrapf(err, "copy bundle file payload")
	}
	return written, nil
}

func cleanupExtractedRoot(root string, committed *bool) {
	if committed != nil && *committed {
		return
	}
	discardError(os.RemoveAll(root))
}
