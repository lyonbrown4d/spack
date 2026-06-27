package spackbundle

import (
	"archive/zip"
	"compress/flate"
	"context"
	"hash/crc32"
	"io"
	"os"
	"strconv"
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/samber/oops"
)

type bundleFilePayload struct {
	file             File
	compressedPath   string
	crc32            uint32
	compressedSize   int64
	uncompressedSize int64
}

type countingWriter struct {
	writer io.Writer
	n      int64
}

func (w *countingWriter) Write(body []byte) (int, error) {
	written, err := w.writer.Write(body)
	w.n += int64(written)
	if err != nil {
		return written, oops.Wrapf(err, "write compressed bundle payload")
	}
	return written, nil
}

func bundleZipSize64(path, label string, size int64) (uint64, error) {
	if size < 0 {
		return 0, oops.Errorf("bundle file %q has negative %s size", path, label)
	}
	parsed, err := strconv.ParseUint(strconv.FormatInt(size, 10), 10, 64)
	if err != nil {
		return 0, oops.Wrapf(err, "convert bundle file %q %s size", path, label)
	}
	return parsed, nil
}

func writeBundleFiles(ctx context.Context, zipWriter *zip.Writer, files []File) (int64, error) {
	payloads, totalBytes, err := prepareBundleFilePayloads(ctx, files)
	defer cleanupBundleFilePayloads(payloads)
	if err != nil {
		return 0, err
	}
	if err := writePreparedBundleFiles(ctx, zipWriter, payloads); err != nil {
		return 0, err
	}
	return totalBytes, nil
}

func prepareBundleFilePayloads(ctx context.Context, files []File) ([]bundleFilePayload, int64, error) {
	payloads := make([]bundleFilePayload, len(files))
	indexes := cxlist.NewListWithCapacity[int](len(files))
	for index := range files {
		indexes.Add(index)
	}
	settings := &asyncx.Settings{Size: bundleFileParallelism(len(files))}
	if err := asyncx.RunList(ctx, nil, settings, "spack_bundle_compress", indexes, func(runCtx context.Context, fileIndex int) error {
		payload, err := prepareBundleFilePayload(runCtx, files[fileIndex])
		if err != nil {
			return err
		}
		payloads[fileIndex] = payload
		return nil
	}); err != nil {
		cleanupBundleFilePayloads(payloads)
		return nil, 0, oops.Wrapf(err, "prepare bundle files")
	}
	return payloads, bundlePayloadTotalBytes(payloads), nil
}

func prepareBundleFilePayload(ctx context.Context, file File) (bundleFilePayload, error) {
	if err := ctx.Err(); err != nil {
		return bundleFilePayload{}, oops.Wrapf(err, "write bundle canceled")
	}
	source, err := os.Open(file.FullPath)
	if err != nil {
		return bundleFilePayload{}, oops.Wrapf(err, "open bundle file %q", file.Path)
	}
	defer func() {
		discardError(source.Close())
	}()

	compressed, err := os.CreateTemp("", "spack-bundle-deflate-*")
	if err != nil {
		return bundleFilePayload{}, oops.Wrapf(err, "create compressed bundle file temp")
	}
	compressedPath := compressed.Name()
	committed := false
	defer cleanupTempPayload(compressed, compressedPath, &committed)

	hasher := crc32.NewIEEE()
	counter := &countingWriter{writer: compressed}
	compressor, err := flate.NewWriter(counter, flate.DefaultCompression)
	if err != nil {
		return bundleFilePayload{}, oops.Wrapf(err, "create bundle file compressor")
	}
	written, err := io.Copy(io.MultiWriter(compressor, hasher), source)
	if err != nil {
		return bundleFilePayload{}, oops.Wrapf(err, "compress bundle file %q", file.Path)
	}
	if err := compressor.Close(); err != nil {
		return bundleFilePayload{}, oops.Wrapf(err, "finish bundle file compression %q", file.Path)
	}
	if err := compressed.Close(); err != nil {
		return bundleFilePayload{}, oops.Wrapf(err, "close compressed bundle file temp")
	}
	if written != file.Size {
		return bundleFilePayload{}, oops.Errorf("bundle file %q size mismatch", file.Path)
	}
	committed = true
	return bundleFilePayload{
		file:             file,
		compressedPath:   compressedPath,
		crc32:            hasher.Sum32(),
		compressedSize:   counter.n,
		uncompressedSize: written,
	}, nil
}

func cleanupTempPayload(file *os.File, path string, committed *bool) {
	discardError(file.Close())
	if committed != nil && *committed {
		return
	}
	discardError(os.Remove(path))
}

func bundlePayloadTotalBytes(payloads []bundleFilePayload) int64 {
	totalBytes := int64(0)
	for index := range payloads {
		totalBytes += payloads[index].uncompressedSize
	}
	return totalBytes
}

func writePreparedBundleFiles(ctx context.Context, zipWriter *zip.Writer, payloads []bundleFilePayload) error {
	for index := range payloads {
		if err := ctx.Err(); err != nil {
			return oops.Wrapf(err, "write bundle canceled")
		}
		if err := writePreparedBundleFile(zipWriter, payloads[index]); err != nil {
			return err
		}
	}
	return nil
}

func writePreparedBundleFile(zipWriter *zip.Writer, payload bundleFilePayload) error {
	compressedSize, err := bundleZipSize64(payload.file.Path, "compressed", payload.compressedSize)
	if err != nil {
		return err
	}
	uncompressedSize, err := bundleZipSize64(payload.file.Path, "uncompressed", payload.uncompressedSize)
	if err != nil {
		return err
	}
	header := &zip.FileHeader{
		Name:               payload.file.Path,
		Method:             zip.Deflate,
		CRC32:              payload.crc32,
		CompressedSize64:   compressedSize,
		UncompressedSize64: uncompressedSize,
	}
	header.SetMode(0o600)
	writer, err := zipWriter.CreateRaw(header)
	if err != nil {
		return oops.Wrapf(err, "create bundle file %q", payload.file.Path)
	}
	source, err := os.Open(payload.compressedPath)
	if err != nil {
		return oops.Wrapf(err, "open compressed bundle file %q", payload.file.Path)
	}
	defer func() {
		discardError(source.Close())
	}()
	written, err := io.Copy(writer, source)
	if err != nil {
		return oops.Wrapf(err, "write bundle file %q", payload.file.Path)
	}
	if written != payload.compressedSize {
		return oops.Errorf("compressed bundle file %q size mismatch", payload.file.Path)
	}
	return nil
}

func cleanupBundleFilePayloads(payloads []bundleFilePayload) {
	for index := range payloads {
		path := strings.TrimSpace(payloads[index].compressedPath)
		if path == "" {
			continue
		}
		discardError(os.Remove(path))
	}
}
