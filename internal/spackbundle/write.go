package spackbundle

import (
	"archive/tar"
	"cmp"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	cxset "github.com/arcgolabs/collectionx/set"
	"github.com/klauspost/compress/zstd"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

// File describes one source file to embed in a bundle.
type File struct {
	Path          string
	FullPath      string
	Kind          string
	Size          int64
	SHA256        string
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
	output, _, files, err := normalizeWriteInputs(options)
	if err != nil {
		return WriteSummary{}, err
	}

	payloads, totalBytes, err := prepareBundleFilePayloads(ctx, files)
	if err != nil {
		return WriteSummary{}, err
	}
	index := buildIndex(payloads, options.Now)
	temp, err := os.CreateTemp(filepath.Dir(output), filepath.Base(output)+".tmp-*")
	if err != nil {
		return WriteSummary{}, oops.Wrapf(err, "create bundle temp file")
	}
	return writeBundleToTemp(ctx, output, temp, index, payloads, totalBytes)
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
	temp *os.File,
	index Index,
	payloads []bundleFilePayload,
	totalBytes int64,
) (WriteSummary, error) {
	tempPath := temp.Name()
	committed := false
	defer cleanupTempBundle(tempPath, &committed)

	if _, err := temp.WriteString(bundleMagic); err != nil {
		return WriteSummary{}, closeBundleFile(temp, oops.Wrapf(err, "write bundle magic"))
	}
	zstdWriter, err := zstd.NewWriter(temp,
		zstd.WithEncoderLevel(zstd.SpeedBestCompression),
		zstd.WithEncoderConcurrency(bundleFileParallelism(len(payloads))),
	)
	if err != nil {
		return WriteSummary{}, closeBundleFile(temp, oops.Wrapf(err, "create bundle zstd writer"))
	}
	tarWriter := tar.NewWriter(zstdWriter)
	if indexErr := writeBundleIndex(tarWriter, index); indexErr != nil {
		return WriteSummary{}, closeBundleWriters(tarWriter, zstdWriter, temp, indexErr)
	}
	if err := writePreparedBundleFiles(ctx, tarWriter, payloads); err != nil {
		return WriteSummary{}, closeBundleWriters(tarWriter, zstdWriter, temp, err)
	}
	if err := tarWriter.Close(); err != nil {
		return WriteSummary{}, closeBundleZstdWriter(zstdWriter, temp, oops.Wrapf(err, "close bundle tar writer"))
	}
	if err := zstdWriter.Close(); err != nil {
		return WriteSummary{}, closeBundleFile(temp, oops.Wrapf(err, "close bundle zstd writer"))
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
		Files:  len(payloads),
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
	seen := cxset.NewSetWithCapacity[string](len(files))
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

func normalizeFile(root string, file File, seen *cxset.Set[string]) (File, error) {
	path, err := cleanBundlePath(file.Path)
	if err != nil {
		return File{}, err
	}
	if seen.Contains(path) {
		return File{}, oops.Errorf("bundle path %q is duplicated", path)
	}
	fullPath, info, err := statBundleFile(root, file)
	if err != nil {
		return File{}, err
	}
	file.Path = path
	file.FullPath = fullPath
	file.Size = info.Size()
	seen.Add(path)
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

func buildIndex(payloads []bundleFilePayload, now func() time.Time) Index {
	if now == nil {
		now = time.Now
	}
	return Index{
		CreatedAt: now().UTC(),
		Files: lo.Map(payloads, func(payload bundleFilePayload, _ int) IndexFile {
			file := payload.file
			return IndexFile{
				Path:       file.Path,
				Kind:       file.Kind,
				Size:       file.Size,
				SHA256:     file.SHA256,
				MediaType:  file.MediaType,
				SourceHash: file.SourceHash,
				ETag:       file.ETag,
				AssetPath:  file.AssetPath,
				Encoding:   file.Encoding,
				Format:     file.Format,
				Width:      file.Width,
			}
		}),
	}
}

func writeBundleIndex(tarWriter *tar.Writer, index Index) error {
	body, err := marshalIndex(index)
	if err != nil {
		return err
	}
	header := &tar.Header{
		Name:     IndexPath,
		Mode:     0o600,
		Size:     int64(len(body)),
		ModTime:  index.CreatedAt,
		Typeflag: tar.TypeReg,
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		return oops.Wrapf(err, "create bundle index")
	}
	if _, err := tarWriter.Write(body); err != nil {
		return oops.Wrapf(err, "write bundle index")
	}
	return nil
}
