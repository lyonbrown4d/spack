package spackbundle

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

// Decompile extracts a SPACK bundle into outputDir for inspection.
func Decompile(ctx context.Context, bundlePath, outputDir string) error {
	_, err := extractTo(ctx, bundlePath, outputDir)
	if err != nil {
		return oops.In("spackbundle").Owner("decompile").With("bundle_path", bundlePath).With("output_dir", outputDir).Wrap(err)
	}
	return nil
}

// ExtractTo extracts a SPACK bundle into outputDir.
func ExtractTo(ctx context.Context, bundlePath, outputDir string) error {
	_, err := extractTo(ctx, bundlePath, outputDir)
	if err != nil {
		return oops.In("spackbundle").Owner("extract").With("bundle_path", bundlePath).With("output_dir", outputDir).Wrap(err)
	}
	return nil
}

// ExtractReadOnly unpacks a SPACK bundle into a temporary read-only directory.
func ExtractReadOnly(ctx context.Context, bundlePath string) (Extracted, error) {
	extracted, err := Extract(ctx, bundlePath)
	if err != nil {
		return Extracted{}, oops.In("spackbundle").Owner("extract readonly").With("bundle_path", bundlePath).Wrap(err)
	}
	if err := makeExtractedTreeReadOnly(extracted.Root); err != nil {
		discardError(extracted.Cleanup())
		return Extracted{}, oops.In("spackbundle").Owner("extract readonly").With("bundle_path", bundlePath).With("root", extracted.Root).Wrap(err)
	}
	return extracted, nil
}

// Extract unpacks a SPACK bundle into a temporary directory.
func Extract(ctx context.Context, bundlePath string) (Extracted, error) {
	absolute, err := normalizedBundlePath(bundlePath)
	if err != nil {
		return Extracted{}, oops.In("spackbundle").Owner("extract").With("bundle_path", bundlePath).Wrap(err)
	}
	root, err := os.MkdirTemp("", "spack-bundle-*")
	if err != nil {
		return Extracted{}, oops.Wrapf(err, "create bundle extraction directory")
	}
	committed := false
	defer cleanupExtractedRoot(root, &committed)

	index, err := extractTo(ctx, absolute, root)
	if err != nil {
		return Extracted{}, oops.In("spackbundle").Owner("extract").With("bundle_path", absolute).With("root", root).Wrap(err)
	}
	committed = true
	return Extracted{BundlePath: absolute, Root: root, Index: index, cleanupRoot: root}, nil
}

// Cleanup removes extracted bundle contents.
func (e Extracted) Cleanup() error {
	cleanupRoot := strings.TrimSpace(e.cleanupRoot)
	if cleanupRoot == "" {
		return nil
	}
	if err := makeExtractedTreeWritable(cleanupRoot); err != nil && !errors.Is(err, os.ErrNotExist) {
		return oops.Wrapf(err, "prepare extracted bundle root cleanup")
	}
	if err := os.RemoveAll(cleanupRoot); err != nil {
		return oops.Wrapf(err, "cleanup extracted bundle root")
	}
	return nil
}

func extractTo(ctx context.Context, bundlePath, outputDir string) (Index, error) {
	root, err := normalizeExtractOutputDir(outputDir)
	if err != nil {
		return Index{}, err
	}
	stream, err := openBundleStream(bundlePath)
	if err != nil {
		return Index{}, err
	}
	defer func() {
		discardError(stream.Close())
	}()
	return extractBundleStream(ctx, stream.tarReader, root)
}

func extractBundleStream(ctx context.Context, reader *tar.Reader, root string) (Index, error) {
	index, expected, err := readBundleIndex(reader)
	if err != nil {
		return Index{}, err
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return Index{}, oops.Wrapf(err, "open extraction root")
	}
	defer func() {
		discardError(rootHandle.Close())
	}()
	if err := extractBundleEntries(ctx, reader, root, rootHandle, expected); err != nil {
		return Index{}, err
	}
	return index, nil
}

func extractBundleEntries(
	ctx context.Context,
	reader *tar.Reader,
	root string,
	rootHandle *os.Root,
	expected map[string]IndexFile,
) error {
	seen := make(map[string]struct{}, len(expected))
	for {
		header, err := nextPayloadHeader(ctx, reader, "extract")
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if err := extractPayloadEntry(root, rootHandle, reader, header, expected, seen); err != nil {
			return err
		}
	}
	return ensureAllIndexFilesSeen(expected, seen)
}

func extractPayloadEntry(
	root string,
	rootHandle *os.Root,
	reader *tar.Reader,
	header *tar.Header,
	expected map[string]IndexFile,
	seen map[string]struct{},
) error {
	entryPath, file, err := lookupPayloadEntry(header, expected, seen)
	if err != nil {
		return err
	}
	if err := extractTarEntry(root, rootHandle, reader, header, file); err != nil {
		return err
	}
	seen[entryPath] = struct{}{}
	return nil
}

func extractTarEntry(root string, rootHandle *os.Root, reader io.Reader, header *tar.Header, expected IndexFile) error {
	if !isRegularTarEntry(header) {
		return oops.Errorf("bundle file %q is not a regular file", expected.Path)
	}
	if header.Size != expected.Size {
		return oops.Errorf("bundle file %q size mismatch", expected.Path)
	}
	target := filepath.Join(root, filepath.FromSlash(expected.Path))
	if !isPathInside(root, target) {
		return oops.Errorf("bundle file %q escapes extraction root", expected.Path)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return oops.Wrapf(err, "create bundle file directory %q", expected.Path)
	}
	return writeExtractedFile(rootHandle, reader, expected)
}

func writeExtractedFile(rootHandle *os.Root, reader io.Reader, expected IndexFile) error {
	targetFile, err := rootHandle.OpenFile(filepath.FromSlash(expected.Path), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return oops.Wrapf(err, "create extracted bundle file %q", expected.Path)
	}
	defer func() {
		discardError(targetFile.Close())
	}()
	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(targetFile, hasher), io.LimitReader(reader, expected.Size+1))
	if err != nil {
		return oops.Wrapf(err, "extract bundle file %q", expected.Path)
	}
	return verifyCopiedPayload(expected, written, hasher.Sum(nil))
}

func normalizeExtractOutputDir(outputDir string) (string, error) {
	outputDir = strings.TrimSpace(outputDir)
	if outputDir == "" {
		return "", oops.In("spackbundle").Owner("extract").Wrap(errors.New("output directory is required"))
	}
	root, err := filepath.Abs(filepath.Clean(outputDir))
	if err != nil {
		return "", oops.Wrapf(err, "resolve output directory")
	}
	return validateOrCreateExtractRoot(root)
}

func validateOrCreateExtractRoot(root string) (string, error) {
	info, err := os.Lstat(root)
	if err == nil {
		return validateExistingExtractRoot(root, info)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", oops.Wrapf(err, "stat output directory")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return "", oops.Wrapf(err, "create output directory")
	}
	return root, nil
}

func validateExistingExtractRoot(root string, info os.FileInfo) (string, error) {
	if info.Mode()&os.ModeSymlink != 0 {
		return "", oops.Errorf("output directory %q is a symlink", root)
	}
	if !info.IsDir() {
		return "", oops.Errorf("output path %q is not a directory", root)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", oops.Wrapf(err, "read output directory")
	}
	if len(entries) > 0 {
		return "", oops.Errorf("output directory %q must be empty", root)
	}
	return root, nil
}

func cleanupExtractedRoot(root string, committed *bool) {
	if committed != nil && *committed {
		return
	}
	discardError(os.RemoveAll(root))
}
