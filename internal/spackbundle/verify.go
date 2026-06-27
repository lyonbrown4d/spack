package spackbundle

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"errors"
	"io"

	"github.com/samber/oops"
)

// Verify validates a SPACK bundle without extracting files.
func Verify(ctx context.Context, bundlePath string) error {
	_, err := verifyBundle(ctx, bundlePath)
	return err
}

func verifyBundle(ctx context.Context, bundlePath string) (Index, error) {
	stream, err := openBundleStream(bundlePath)
	if err != nil {
		return Index{}, err
	}
	defer func() {
		discardError(stream.Close())
	}()
	index, expected, err := readBundleIndex(stream.tarReader)
	if err != nil {
		return Index{}, err
	}
	if err := verifyBundleEntries(ctx, stream.tarReader, expected); err != nil {
		return Index{}, err
	}
	return index, nil
}

func verifyBundleEntries(ctx context.Context, reader *tar.Reader, expected map[string]IndexFile) error {
	seen := make(map[string]struct{}, len(expected))
	for {
		header, err := nextPayloadHeader(ctx, reader, "verify")
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if err := verifyPayloadEntry(reader, header, expected, seen); err != nil {
			return err
		}
	}
	return ensureAllIndexFilesSeen(expected, seen)
}

func verifyPayloadEntry(
	reader *tar.Reader,
	header *tar.Header,
	expected map[string]IndexFile,
	seen map[string]struct{},
) error {
	entryPath, file, err := lookupPayloadEntry(header, expected, seen)
	if err != nil {
		return err
	}
	if err := verifyTarEntry(reader, header, file); err != nil {
		return err
	}
	seen[entryPath] = struct{}{}
	return nil
}

func verifyTarEntry(reader io.Reader, header *tar.Header, expected IndexFile) error {
	if !isRegularTarEntry(header) {
		return oops.Errorf("bundle file %q is not a regular file", expected.Path)
	}
	if header.Size != expected.Size {
		return oops.Errorf("bundle file %q size mismatch", expected.Path)
	}
	hasher := sha256.New()
	written, err := io.Copy(hasher, io.LimitReader(reader, expected.Size+1))
	if err != nil {
		return oops.Wrapf(err, "read bundle file %q", expected.Path)
	}
	return verifyCopiedPayload(expected, written, hasher.Sum(nil))
}

func nextPayloadHeader(ctx context.Context, reader *tar.Reader, action string) (*tar.Header, error) {
	if err := ctx.Err(); err != nil {
		return nil, oops.Wrapf(err, "%s bundle canceled", action)
	}
	header, err := reader.Next()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, io.EOF
		}
		return nil, oops.Wrapf(err, "read bundle tar entry")
	}
	return header, nil
}

func lookupPayloadEntry(
	header *tar.Header,
	expected map[string]IndexFile,
	seen map[string]struct{},
) (string, IndexFile, error) {
	entryPath, err := cleanTarEntryPath(header)
	if err != nil {
		return "", IndexFile{}, err
	}
	file, ok := expected[entryPath]
	if !ok {
		return "", IndexFile{}, oops.Errorf("bundle file %q is not declared in index", entryPath)
	}
	if _, exists := seen[entryPath]; exists {
		return "", IndexFile{}, oops.Errorf("bundle file %q is duplicated", entryPath)
	}
	return entryPath, file, nil
}
