package spackbundle

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"path"
	"path/filepath"

	"github.com/samber/oops"
)

func readBundleIndex(reader *tar.Reader) (Index, map[string]IndexFile, error) {
	header, err := reader.Next()
	if err != nil {
		return Index{}, nil, oops.Wrapf(err, "read bundle index entry")
	}
	entryPath, err := cleanTarEntryPath(header)
	if err != nil {
		return Index{}, nil, err
	}
	if entryPath != IndexPath {
		return Index{}, nil, oops.Errorf("first bundle entry must be %q, got %q", IndexPath, entryPath)
	}
	body, err := readTarEntryBody(reader, header, entryPath)
	if err != nil {
		return Index{}, nil, err
	}
	index, err := unmarshalIndex(body)
	if err != nil {
		return Index{}, nil, err
	}
	if err := validateIndex(index); err != nil {
		return Index{}, nil, err
	}
	return index, indexFileMap(index), nil
}

func validateIndex(index Index) error {
	seen := make(map[string]struct{}, len(index.Files))
	for i := range index.Files {
		if err := validateIndexFile(index.Files[i], seen); err != nil {
			return err
		}
	}
	return nil
}

func validateIndexFile(file IndexFile, seen map[string]struct{}) error {
	cleaned, err := cleanBundlePath(file.Path)
	if err != nil {
		return err
	}
	if cleaned != file.Path {
		return oops.Errorf("bundle index path %q is not normalized", file.Path)
	}
	if file.Size < 0 || file.Size > maxExtractedFileBytes {
		return oops.Errorf("bundle file %q exceeds max extracted bytes", file.Path)
	}
	if err := validateIndexFileHash(file); err != nil {
		return err
	}
	if _, ok := seen[file.Path]; ok {
		return oops.Errorf("bundle index path %q is duplicated", file.Path)
	}
	seen[file.Path] = struct{}{}
	return nil
}

func validateIndexFileHash(file IndexFile) error {
	sha, err := hex.DecodeString(file.SHA256)
	if err != nil || len(sha) != sha256.Size {
		return oops.Errorf("bundle file %q has invalid sha256", file.Path)
	}
	return nil
}

func indexFileMap(index Index) map[string]IndexFile {
	files := make(map[string]IndexFile, len(index.Files))
	for i := range index.Files {
		files[index.Files[i].Path] = index.Files[i]
	}
	return files
}

func cleanTarEntryPath(header *tar.Header) (string, error) {
	if header == nil {
		return "", oops.In("spackbundle").Owner("read").Wrap(errors.New("bundle tar entry is nil"))
	}
	cleaned := path.Clean(filepath.ToSlash(header.Name))
	if cleaned == IndexPath {
		return cleaned, nil
	}
	if isMetadataPath(cleaned) {
		return "", oops.Errorf("bundle file %q uses reserved metadata namespace", header.Name)
	}
	return cleanBundlePath(cleaned)
}

func isRegularTarEntry(header *tar.Header) bool {
	return header.Typeflag == tar.TypeReg
}

func readTarEntryBody(reader *tar.Reader, header *tar.Header, filePath string) ([]byte, error) {
	if !isRegularTarEntry(header) {
		return nil, oops.Errorf("bundle file %q is not a regular file", filePath)
	}
	if header.Size < 0 || header.Size > maxExtractedFileBytes {
		return nil, oops.Errorf("bundle file %q exceeds max extracted bytes", filePath)
	}
	limited := io.LimitReader(reader, header.Size+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, oops.Wrapf(err, "read bundle file %q", filePath)
	}
	if int64(len(body)) != header.Size {
		return nil, oops.Errorf("bundle file %q size mismatch", filePath)
	}
	return body, nil
}

func verifyCopiedPayload(expected IndexFile, written int64, digest []byte) error {
	if written != expected.Size {
		return oops.Errorf("bundle file %q size mismatch", expected.Path)
	}
	if got := hex.EncodeToString(digest); got != expected.SHA256 {
		return oops.Errorf("bundle file %q sha256 mismatch", expected.Path)
	}
	return nil
}

func verifyBody(expected IndexFile, body []byte) error {
	digest := sha256.Sum256(body)
	return verifyCopiedPayload(expected, int64(len(body)), digest[:])
}

func ensureAllIndexFilesSeen(expected map[string]IndexFile, seen map[string]struct{}) error {
	for filePath := range expected {
		if _, ok := seen[filePath]; !ok {
			return oops.Errorf("bundle file %q is missing", filePath)
		}
	}
	return nil
}
