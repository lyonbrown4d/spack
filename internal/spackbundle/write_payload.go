package spackbundle

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/samber/oops"
)

type bundleFilePayload struct {
	file File
}

func prepareBundleFilePayloads(ctx context.Context, files []File) ([]bundleFilePayload, int64, error) {
	payloads := make([]bundleFilePayload, len(files))
	indexes := cxlist.NewListWithCapacity[int](len(files))
	for index := range files {
		indexes.Add(index)
	}
	settings := &asyncx.Settings{Size: bundleFileParallelism(len(files))}
	if err := asyncx.RunList(ctx, nil, settings, "spack_bundle_hash", indexes, func(runCtx context.Context, fileIndex int) error {
		payload, err := prepareBundleFilePayload(runCtx, files[fileIndex])
		if err != nil {
			return err
		}
		payloads[fileIndex] = payload
		return nil
	}); err != nil {
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

	hasher := sha256.New()
	written, err := io.Copy(hasher, source)
	if err != nil {
		return bundleFilePayload{}, oops.Wrapf(err, "hash bundle file %q", file.Path)
	}
	if written != file.Size {
		return bundleFilePayload{}, oops.Errorf("bundle file %q size mismatch", file.Path)
	}
	file.SHA256 = hex.EncodeToString(hasher.Sum(nil))
	return bundleFilePayload{file: file}, nil
}

func bundlePayloadTotalBytes(payloads []bundleFilePayload) int64 {
	totalBytes := int64(0)
	for index := range payloads {
		totalBytes += payloads[index].file.Size
	}
	return totalBytes
}

func writePreparedBundleFiles(ctx context.Context, tarWriter *tar.Writer, payloads []bundleFilePayload) error {
	for index := range payloads {
		if err := ctx.Err(); err != nil {
			return oops.Wrapf(err, "write bundle canceled")
		}
		if err := writePreparedBundleFile(tarWriter, payloads[index]); err != nil {
			return err
		}
	}
	return nil
}

func writePreparedBundleFile(tarWriter *tar.Writer, payload bundleFilePayload) error {
	file := payload.file
	header := &tar.Header{
		Name:     file.Path,
		Mode:     0o600,
		Size:     file.Size,
		Typeflag: tar.TypeReg,
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		return oops.Wrapf(err, "create bundle file %q", file.Path)
	}
	source, err := os.Open(file.FullPath)
	if err != nil {
		return oops.Wrapf(err, "open bundle file %q", file.Path)
	}
	defer func() {
		discardError(source.Close())
	}()
	written, err := io.Copy(tarWriter, source)
	if err != nil {
		return oops.Wrapf(err, "write bundle file %q", file.Path)
	}
	if written != file.Size {
		return oops.Errorf("bundle file %q size mismatch", file.Path)
	}
	return nil
}
