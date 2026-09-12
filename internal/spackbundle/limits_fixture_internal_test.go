package spackbundle

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

type limitBundleEntry struct {
	path string
	body []byte
}

func limitTestLimits(files int, expandedBytes, fileBytes, indexBytes int64) bundleLimits {
	return bundleLimits{
		indexBytes:    indexBytes,
		files:         files,
		expandedBytes: expandedBytes,
		fileBytes:     fileBytes,
	}
}

func limitIndexFile(path string, body []byte) IndexFile {
	digest := sha256.Sum256(body)
	return IndexFile{
		Path:   path,
		Kind:   "asset",
		Size:   int64(len(body)),
		SHA256: hex.EncodeToString(digest[:]),
	}
}

func writeLimitBundle(
	t *testing.T,
	files []IndexFile,
	entries []limitBundleEntry,
	limits bundleLimits,
) string {
	t.Helper()

	file := createLimitBundleFile(t)
	encoder, tarWriter := openLimitBundleWriters(t, file)
	writeLimitBundleIndex(t, tarWriter, files, limits)
	writeLimitBundleEntries(t, tarWriter, entries)
	closeLimitBundleWriters(t, file, encoder, tarWriter)
	return file.Name()
}

func createLimitBundleFile(t *testing.T) *os.File {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "limit-*.spack")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(bundleMagic); err != nil {
		t.Fatal(err)
	}
	return file
}

func openLimitBundleWriters(t *testing.T, file *os.File) (*zstd.Encoder, *tar.Writer) {
	t.Helper()
	encoder, err := zstd.NewWriter(file)
	if err != nil {
		t.Fatal(err)
	}
	return encoder, tar.NewWriter(encoder)
}

func writeLimitBundleIndex(
	t *testing.T,
	writer *tar.Writer,
	files []IndexFile,
	limits bundleLimits,
) {
	t.Helper()
	index := Index{CreatedAt: time.Unix(1, 0).UTC(), Files: files}
	if err := writeBundleIndex(writer, index, limits); err != nil {
		t.Fatal(err)
	}
}

func writeLimitBundleEntries(t *testing.T, writer *tar.Writer, entries []limitBundleEntry) {
	t.Helper()
	for _, entry := range entries {
		writeLimitBundleEntry(t, writer, entry)
	}
}

func writeLimitBundleEntry(t *testing.T, writer *tar.Writer, entry limitBundleEntry) {
	t.Helper()
	if err := writer.WriteHeader(&tar.Header{
		Name:     entry.path,
		Mode:     0o600,
		Size:     int64(len(entry.body)),
		ModTime:  time.Unix(1, 0).UTC(),
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(entry.body); err != nil {
		t.Fatal(err)
	}
}

func closeLimitBundleWriters(
	t *testing.T,
	file *os.File,
	encoder *zstd.Encoder,
	writer *tar.Writer,
) {
	t.Helper()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func readIndexWithLimits(bundlePath string, limits bundleLimits) (Index, error) {
	stream, err := openBundleStream(bundlePath)
	if err != nil {
		return Index{}, err
	}
	defer func() {
		discardError(stream.Close())
	}()
	index, _, err := readBundleIndex(stream.tarReader, limits)
	return index, err
}

func assertLimitError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error containing %q, got %v", want, err)
	}
}
