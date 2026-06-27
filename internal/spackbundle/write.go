package spackbundle

import (
	"archive/zip"
	"cmp"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/samber/oops"
)

// File describes one source file to embed in a bundle.
type File struct {
	Path          string
	FullPath      string
	Kind          string
	Size          int64
	MediaType     string
	SourceHash    string
	ETag          string
	AssetPath     string
	Encoding      string
	Format        string
	Width         int
	AllowExternal bool
}

// WriteOptions configures bundle writing.
type WriteOptions struct {
	Output string
	Root   string
	Files  []File
	Now    func() time.Time
}

// WriteSummary describes a completed bundle write.
type WriteSummary struct {
	Output string
	Files  int
	Bytes  int64
}

// Write creates a SPACK bundle at options.Output.
func Write(ctx context.Context, options WriteOptions) (WriteSummary, error) {
	output, root, files, err := normalizeWriteInputs(options)
	if err != nil {
		return WriteSummary{}, err
	}

	index := buildIndex(files, options.Now)
	temp, err := os.CreateTemp(filepath.Dir(output), filepath.Base(output)+".tmp-*")
	if err != nil {
		return WriteSummary{}, oops.Wrapf(err, "create bundle temp file")
	}
	return writeBundleToTemp(ctx, output, root, temp, index, files)
}

func normalizeWriteInputs(options WriteOptions) (string, string, []File, error) {
	output, err := normalizedOutputPath(options.Output)
	if err != nil {
		return "", "", nil, err
	}
	root, err := normalizedRootPath(options.Root)
	if err != nil {
		return "", "", nil, err
	}
	files, err := normalizeFiles(root, options.Files)
	if err != nil {
		return "", "", nil, err
	}
	return output, root, files, nil
}

func writeBundleToTemp(
	ctx context.Context,
	output string,
	root string,
	temp *os.File,
	index Index,
	files []File,
) (WriteSummary, error) {
	_ = root
	tempPath := temp.Name()
	committed := false
	defer cleanupTempBundle(tempPath, &committed)

	zipWriter := zip.NewWriter(temp)
	if indexErr := writeBundleIndex(zipWriter, index); indexErr != nil {
		return WriteSummary{}, closeBundleWriters(zipWriter, temp, indexErr)
	}
	totalBytes, err := writeBundleFiles(ctx, zipWriter, files)
	if err != nil {
		return WriteSummary{}, closeBundleWriters(zipWriter, temp, err)
	}
	if err := zipWriter.Close(); err != nil {
		return WriteSummary{}, closeBundleFile(temp, oops.Wrapf(err, "close bundle zip writer"))
	}
	if err := temp.Close(); err != nil {
		return WriteSummary{}, oops.Wrapf(err, "close bundle temp file")
	}
	if err := os.Chmod(tempPath, bundleOutputFileMode()); err != nil {
		return WriteSummary{}, oops.Wrapf(err, "set bundle file mode")
	}
	if err := os.Rename(tempPath, output); err != nil {
		return WriteSummary{}, oops.Wrapf(err, "publish bundle")
	}
	committed = true

	return WriteSummary{
		Output: output,
		Files:  len(files),
		Bytes:  totalBytes,
	}, nil
}

func bundleOutputFileMode() os.FileMode {
	return os.FileMode(0o600) | os.FileMode(0o044)
}

func normalizedOutputPath(output string) (string, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return "", oops.In("spackbundle").Owner("write").Wrap(errors.New("bundle output path is required"))
	}
	absolute, err := filepath.Abs(filepath.Clean(output))
	if err != nil {
		return "", oops.Wrapf(err, "resolve bundle output path")
	}
	return absolute, nil
}

func normalizedRootPath(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", oops.In("spackbundle").Owner("write").Wrap(errors.New("bundle root path is required"))
	}
	absolute, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", oops.Wrapf(err, "resolve bundle root path")
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", oops.Wrapf(err, "stat bundle root")
	}
	if !info.IsDir() {
		return "", oops.Errorf("bundle root must be a directory: %s", absolute)
	}
	return absolute, nil
}

func normalizeFiles(root string, files []File) ([]File, error) {
	normalized := make([]File, 0, len(files))
	seen := map[string]struct{}{}
	for index := range files {
		file, err := normalizeFile(root, files[index], seen)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, file)
	}
	slices.SortFunc(normalized, func(left, right File) int {
		return cmp.Compare(left.Path, right.Path)
	})
	return normalized, nil
}

func normalizeFile(root string, file File, seen map[string]struct{}) (File, error) {
	path, err := cleanBundlePath(file.Path)
	if err != nil {
		return File{}, err
	}
	if _, ok := seen[path]; ok {
		return File{}, oops.Errorf("bundle path %q is duplicated", path)
	}
	fullPath, info, err := statBundleFile(root, file)
	if err != nil {
		return File{}, err
	}
	file.Path = path
	file.FullPath = fullPath
	file.Size = info.Size()
	seen[path] = struct{}{}
	return file, nil
}

func statBundleFile(root string, file File) (string, os.FileInfo, error) {
	fullPath, err := filepath.Abs(filepath.Clean(file.FullPath))
	if err != nil {
		return "", nil, oops.Wrapf(err, "resolve bundle file %q", file.Path)
	}
	if !file.AllowExternal && !isPathInside(root, fullPath) {
		return "", nil, oops.Errorf("bundle file %q escapes root", file.Path)
	}
	info, err := os.Lstat(fullPath)
	if err != nil {
		return "", nil, oops.Wrapf(err, "stat bundle file %q", file.Path)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", nil, oops.Errorf("bundle file %q is a symlink", file.Path)
	}
	if info.IsDir() {
		return "", nil, oops.Errorf("bundle file %q is a directory", file.Path)
	}
	return fullPath, info, nil
}

func buildIndex(files []File, now func() time.Time) Index {
	if now == nil {
		now = time.Now
	}
	index := Index{
		CreatedAt: now().UTC(),
		Files:     make([]IndexFile, 0, len(files)),
	}
	for fileIndex := range files {
		file := files[fileIndex]
		index.Files = append(index.Files, IndexFile{
			Path:       file.Path,
			Kind:       file.Kind,
			Size:       file.Size,
			MediaType:  file.MediaType,
			SourceHash: file.SourceHash,
			ETag:       file.ETag,
			AssetPath:  file.AssetPath,
			Encoding:   file.Encoding,
			Format:     file.Format,
			Width:      file.Width,
		})
	}
	return index
}

func writeBundleIndex(zipWriter *zip.Writer, index Index) error {
	body, err := marshalIndex(index)
	if err != nil {
		return err
	}
	header := &zip.FileHeader{
		Name:   IndexPath,
		Method: zip.Deflate,
	}
	header.SetMode(0o600)
	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return oops.Wrapf(err, "create bundle index")
	}
	if _, err := writer.Write(body); err != nil {
		return oops.Wrapf(err, "write bundle index")
	}
	return nil
}
