package spackbundle

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/samber/lo"
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
	source, info, err := openBundleSourceFile(file)
	if err != nil {
		return bundleFilePayload{}, oops.Wrapf(err, "open bundle file %q", file.Path)
	}
	defer discardClose(source)
	if info.Size() != file.Size {
		return bundleFilePayload{}, oops.Errorf("bundle file %q size mismatch", file.Path)
	}

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
	return lo.SumBy(payloads, func(payload bundleFilePayload) int64 {
		return payload.file.Size
	})
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
	source, info, err := openBundleSourceFile(file)
	if err != nil {
		return oops.Wrapf(err, "open bundle file %q", file.Path)
	}
	defer discardClose(source)
	if info.Size() != file.Size {
		return oops.Errorf("bundle file %q size mismatch", file.Path)
	}
	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(tarWriter, hasher), source)
	if err != nil {
		return oops.Wrapf(err, "write bundle file %q", file.Path)
	}
	if written != file.Size {
		return oops.Errorf("bundle file %q size mismatch", file.Path)
	}
	if got := hex.EncodeToString(hasher.Sum(nil)); got != file.SHA256 {
		return oops.Errorf("bundle file %q sha256 changed during write", file.Path)
	}
	return nil
}

func openBundleSourceFile(file File) (*os.File, fs.FileInfo, error) {
	if !file.AllowExternal && file.root != "" && file.rootRelativePath != "" {
		return openRootBundleFile(file)
	}
	return openExternalBundleFile(file.FullPath)
}

func openRootBundleFile(file File) (*os.File, fs.FileInfo, error) {
	rootDir, err := openBundleRoot(file.root, file.rootInfo)
	if err != nil {
		return nil, nil, err
	}
	defer discardBundleRoot(rootDir)
	info, err := lstatBundlePathWithinRoot(rootDir, file.root, file.rootRelativePath)
	if err != nil {
		return nil, nil, err
	}
	opened, err := rootDir.Open(filepath.FromSlash(file.rootRelativePath))
	if err != nil {
		return nil, nil, oops.Wrap(err)
	}
	openedInfo, err := opened.Stat()
	if err != nil {
		discardClose(opened)
		return nil, nil, oops.Wrap(err)
	}
	if openedInfo.IsDir() || !os.SameFile(info, openedInfo) {
		discardClose(opened)
		return nil, nil, oops.In("spackbundle").Owner("write").Wrap(errors.New("bundle source changed during open"))
	}
	return opened, openedInfo, nil
}

func openBundleRoot(root string, expected fs.FileInfo) (*os.Root, error) {
	rootDir, err := os.OpenRoot(root)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	openedInfo, err := rootDir.Stat(".")
	if err != nil {
		discardBundleRoot(rootDir)
		return nil, oops.Wrap(err)
	}
	currentInfo, err := os.Lstat(root)
	if err != nil {
		discardBundleRoot(rootDir)
		return nil, oops.Wrap(err)
	}
	if currentInfo.Mode()&os.ModeSymlink != 0 || !currentInfo.IsDir() || !os.SameFile(openedInfo, currentInfo) {
		discardBundleRoot(rootDir)
		return nil, oops.In("spackbundle").Owner("write").Wrap(errors.New("bundle root was replaced"))
	}
	if expected != nil && !os.SameFile(expected, currentInfo) {
		discardBundleRoot(rootDir)
		return nil, oops.In("spackbundle").Owner("write").Wrap(errors.New("bundle root was replaced"))
	}
	return rootDir, nil
}

func openExternalBundleFile(fullPath string) (*os.File, fs.FileInfo, error) {
	absolute, err := filepath.Abs(filepath.Clean(fullPath))
	if err != nil {
		return nil, nil, oops.Wrap(err)
	}
	parent := filepath.Dir(absolute)
	name := filepath.Base(absolute)
	rootDir, err := os.OpenRoot(parent)
	if err != nil {
		return nil, nil, oops.Wrap(err)
	}
	defer discardBundleRoot(rootDir)
	info, err := rootDir.Lstat(name)
	if err != nil {
		return nil, nil, oops.Wrap(err)
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
		return nil, nil, oops.In("spackbundle").Owner("write").Wrap(errors.New("bundle external source is not a regular file"))
	}
	opened, err := rootDir.Open(name)
	if err != nil {
		return nil, nil, oops.Wrap(err)
	}
	openedInfo, err := opened.Stat()
	if err != nil {
		discardClose(opened)
		return nil, nil, oops.Wrap(err)
	}
	if !os.SameFile(info, openedInfo) {
		discardClose(opened)
		return nil, nil, oops.In("spackbundle").Owner("write").Wrap(errors.New("bundle external source changed during open"))
	}
	return opened, openedInfo, nil
}

func discardClose(file *os.File) {
	if file == nil {
		return
	}
	if err := file.Close(); err != nil {
		return
	}
}
