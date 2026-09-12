package spackbundle

import (
	"archive/tar"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

func TestReaderAcceptsExactExpandedByteLimit(t *testing.T) {
	t.Parallel()

	exactBundle := writeDeclaredIndexBundle(t, declaredFilesAtExpandedLimit())
	reader, err := OpenReader(exactBundle)
	if err != nil {
		t.Fatal(err)
	}
	defer closeLimitTestReader(t, reader)

	if _, indexErr := reader.Index(); indexErr != nil {
		t.Fatalf("expected reader index at exact limit to be accepted: %v", indexErr)
	}
}

func TestReaderRejectsExpandedByteLimit(t *testing.T) {
	t.Parallel()

	overLimitFiles := append(declaredFilesAtExpandedLimit(), declaredIndexFile("assets/overflow.bin", 1))
	reader, err := OpenReader(writeDeclaredIndexBundle(t, overLimitFiles))
	if err != nil {
		t.Fatal(err)
	}
	defer closeLimitTestReader(t, reader)

	_, err = reader.Index()
	assertExpandedLimitError(t, err)
}

func TestVerifyReachesPayloadAtExactExpandedByteLimit(t *testing.T) {
	t.Parallel()

	err := Verify(t.Context(), writeDeclaredIndexBundle(t, declaredFilesAtExpandedLimit()))
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected exact limit index to reach missing-payload validation, got %v", err)
	}
}

func TestVerifyRejectsExpandedByteLimit(t *testing.T) {
	t.Parallel()

	overLimitFiles := append(declaredFilesAtExpandedLimit(), declaredIndexFile("assets/overflow.bin", 1))
	assertExpandedLimitError(t, Verify(t.Context(), writeDeclaredIndexBundle(t, overLimitFiles)))
}

func TestExtractReachesPayloadAtExactExpandedByteLimit(t *testing.T) {
	t.Parallel()

	err := ExtractTo(
		t.Context(),
		writeDeclaredIndexBundle(t, declaredFilesAtExpandedLimit()),
		filepath.Join(t.TempDir(), "exact"),
	)
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected exact limit index to reach missing-payload validation, got %v", err)
	}
}

func TestExtractRejectsExpandedByteLimit(t *testing.T) {
	t.Parallel()

	overLimitFiles := append(declaredFilesAtExpandedLimit(), declaredIndexFile("assets/overflow.bin", 1))
	assertExpandedLimitError(t, ExtractTo(
		t.Context(),
		writeDeclaredIndexBundle(t, overLimitFiles),
		filepath.Join(t.TempDir(), "over"),
	))
}

func declaredFilesAtExpandedLimit() []IndexFile {
	files := make([]IndexFile, 0, maxExpandedBundleBytes/maxExtractedFileBytes)
	for index := range maxExpandedBundleBytes / maxExtractedFileBytes {
		files = append(files, declaredIndexFile(fmt.Sprintf("assets/%d.bin", index), maxExtractedFileBytes))
	}
	return files
}

func declaredIndexFile(path string, size int64) IndexFile {
	return IndexFile{
		Path:   path,
		Kind:   "asset",
		Size:   size,
		SHA256: strings.Repeat("0", sha256.Size*2),
	}
}

func writeDeclaredIndexBundle(t *testing.T, files []IndexFile) string {
	t.Helper()

	file, err := os.CreateTemp(t.TempDir(), "declared-*.spack")
	if err != nil {
		t.Fatal(err)
	}
	path := file.Name()
	if _, writeErr := file.WriteString(bundleMagic); writeErr != nil {
		t.Fatal(writeErr)
	}
	encoder, err := zstd.NewWriter(file)
	if err != nil {
		t.Fatal(err)
	}
	tarWriter := tar.NewWriter(encoder)
	index := Index{CreatedAt: time.Unix(1, 0).UTC(), Files: files}
	if err := writeBundleIndex(tarWriter, index, productionBundleLimits); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func closeLimitTestReader(t *testing.T, reader *Reader) {
	t.Helper()
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertExpandedLimitError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected expanded byte limit to be rejected")
	}
	if !strings.Contains(err.Error(), "bundle exceeds max expanded bytes") {
		t.Fatalf("unexpected error: %v", err)
	}
}
