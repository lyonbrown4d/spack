package spackbundle

import (
	"archive/tar"
	"context"
	"github.com/klauspost/compress/zstd"
	"github.com/samber/lo"
	"github.com/samber/oops"
	"io/fs"
	"os"
	"path/filepath"
	"time"
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

	root             string
	rootInfo         fs.FileInfo
	rootRelativePath string
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
	root, rootInfo, err := normalizedRootPath(options.Root)
	if err != nil {
		return "", "", nil, err
	}
	files, err := normalizeFiles(root, rootInfo, options.Files)
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
