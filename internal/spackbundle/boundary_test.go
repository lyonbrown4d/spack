package spackbundle_test

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

type boundaryBundleCase struct {
	name    string
	files   []spackbundle.IndexFile
	entries []boundaryBundleEntry
	want    string
}

type boundaryBundleEntry struct {
	path     string
	body     []byte
	typeflag byte
}

func TestVerifyRejectsBundlePayloadBoundaries(t *testing.T) {
	for _, tc := range boundaryPayloadCases() {
		t.Run(tc.name, func(t *testing.T) {
			bundlePath := writeBoundaryBundle(t, tc.files, tc.entries)

			err := spackbundle.Verify(context.Background(), bundlePath)
			if err == nil {
				t.Fatal("expected verify to reject boundary bundle")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestExtractRejectsEscapingPayloadEntry(t *testing.T) {
	body := []byte("console.log('ok');")
	bundlePath := writeBoundaryBundle(t, []spackbundle.IndexFile{boundaryIndexFile("assets/app.js", body)}, []boundaryBundleEntry{
		{path: "../app.js", body: body, typeflag: tar.TypeReg},
	})

	_, err := spackbundle.Extract(context.Background(), bundlePath)
	if err == nil {
		t.Fatal("expected extract to reject escaping payload entry")
	}
	if !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("expected escaping error, got %v", err)
	}
}

func TestOpenReaderRejectsWrongBundleMagic(t *testing.T) {
	bundlePath := filepath.Join(t.TempDir(), "bad.spack")
	if err := os.WriteFile(bundlePath, []byte("not-a-spack-bundle"), 0o600); err != nil {
		t.Fatal(err)
	}

	reader, err := spackbundle.OpenReader(bundlePath)
	if err == nil {
		if closeErr := reader.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		t.Fatal("expected reader to reject wrong bundle magic")
	}
	if !strings.Contains(err.Error(), "bundle magic") {
		t.Fatalf("expected bundle magic error, got %v", err)
	}
}

func boundaryPayloadCases() []boundaryBundleCase {
	body := []byte("console.log('ok');")
	declared := boundaryIndexFile("assets/app.js", body)
	return []boundaryBundleCase{
		boundaryMissingFileCase(declared),
		boundaryUndeclaredFileCase(declared, body),
		boundaryDuplicatedFileCase(declared, body),
		boundaryDirectoryFileCase(declared),
		boundarySizeMismatchCase(declared, body),
		boundaryShaMismatchCase(declared, body),
	}
}

func boundaryMissingFileCase(declared spackbundle.IndexFile) boundaryBundleCase {
	return boundaryBundleCase{name: "missing declared file", files: []spackbundle.IndexFile{declared}, want: "missing"}
}

func boundaryUndeclaredFileCase(declared spackbundle.IndexFile, body []byte) boundaryBundleCase {
	return boundaryBundleCase{
		name:  "undeclared payload file",
		files: []spackbundle.IndexFile{declared},
		entries: []boundaryBundleEntry{
			{path: "assets/app.js", body: body, typeflag: tar.TypeReg},
			{path: "assets/extra.js", body: []byte("extra"), typeflag: tar.TypeReg},
		},
		want: "not declared",
	}
}

func boundaryDuplicatedFileCase(declared spackbundle.IndexFile, body []byte) boundaryBundleCase {
	return boundaryBundleCase{
		name:  "duplicated payload file",
		files: []spackbundle.IndexFile{declared},
		entries: []boundaryBundleEntry{
			{path: "assets/app.js", body: body, typeflag: tar.TypeReg},
			{path: "assets/app.js", body: body, typeflag: tar.TypeReg},
		},
		want: "duplicated",
	}
}

func boundaryDirectoryFileCase(declared spackbundle.IndexFile) boundaryBundleCase {
	return boundaryBundleCase{
		name:    "directory payload file",
		files:   []spackbundle.IndexFile{declared},
		entries: []boundaryBundleEntry{{path: "assets/app.js", typeflag: tar.TypeDir}},
		want:    "not a regular file",
	}
}

func boundarySizeMismatchCase(declared spackbundle.IndexFile, body []byte) boundaryBundleCase {
	file := declared
	file.Size++
	return boundaryBundleCase{
		name:    "payload size mismatch",
		files:   []spackbundle.IndexFile{file},
		entries: []boundaryBundleEntry{{path: "assets/app.js", body: body, typeflag: tar.TypeReg}},
		want:    "size mismatch",
	}
}

func boundaryShaMismatchCase(declared spackbundle.IndexFile, body []byte) boundaryBundleCase {
	file := declared
	file.SHA256 = strings.Repeat("0", sha256.Size*2)
	return boundaryBundleCase{
		name:    "payload sha mismatch",
		files:   []spackbundle.IndexFile{file},
		entries: []boundaryBundleEntry{{path: "assets/app.js", body: body, typeflag: tar.TypeReg}},
		want:    "sha256 mismatch",
	}
}

func writeBoundaryBundle(t *testing.T, files []spackbundle.IndexFile, entries []boundaryBundleEntry) string {
	t.Helper()

	file := createBoundaryBundleFile(t)
	encoder, tarWriter := openBoundaryBundleWriters(t, file)
	writeBoundaryIndexEntry(t, tarWriter, files)
	writeBoundaryPayloadEntries(t, tarWriter, entries)
	closeBoundaryBundleWriters(t, file, encoder, tarWriter)
	return file.Name()
}

func createBoundaryBundleFile(t *testing.T) *os.File {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "app-*.spack")
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func openBoundaryBundleWriters(t *testing.T, file *os.File) (*zstd.Encoder, *tar.Writer) {
	t.Helper()
	writeBoundaryMagic(t, file)
	encoder, err := zstd.NewWriter(file)
	if err != nil {
		t.Fatal(err)
	}
	return encoder, tar.NewWriter(encoder)
}

func writeBoundaryMagic(t *testing.T, file *os.File) {
	t.Helper()
	if _, err := file.WriteString("SPACKBND\x00"); err != nil {
		t.Fatal(err)
	}
}

func writeBoundaryIndexEntry(t *testing.T, tarWriter *tar.Writer, files []spackbundle.IndexFile) {
	t.Helper()
	indexBody := marshalBoundaryIndex(t, files)
	writeBoundaryTarEntry(t, tarWriter, boundaryBundleEntry{
		path:     spackbundle.IndexPath,
		body:     indexBody,
		typeflag: tar.TypeReg,
	})
}

func writeBoundaryPayloadEntries(t *testing.T, tarWriter *tar.Writer, entries []boundaryBundleEntry) {
	t.Helper()
	for _, entry := range entries {
		writeBoundaryTarEntry(t, tarWriter, entry)
	}
}

func writeBoundaryTarEntry(t *testing.T, tarWriter *tar.Writer, entry boundaryBundleEntry) {
	t.Helper()
	header := boundaryTarHeader(entry)
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if header.Typeflag != tar.TypeReg {
		return
	}
	if _, err := tarWriter.Write(entry.body); err != nil {
		t.Fatal(err)
	}
}

func boundaryTarHeader(entry boundaryBundleEntry) *tar.Header {
	typeflag := entry.typeflag
	if typeflag == 0 {
		typeflag = tar.TypeReg
	}
	header := &tar.Header{
		Name:     entry.path,
		Mode:     0o600,
		Size:     int64(len(entry.body)),
		ModTime:  time.Unix(1, 0).UTC(),
		Typeflag: typeflag,
	}
	if typeflag != tar.TypeReg {
		header.Size = 0
	}
	return header
}

func closeBoundaryBundleWriters(t *testing.T, file *os.File, encoder *zstd.Encoder, tarWriter *tar.Writer) {
	t.Helper()
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func marshalBoundaryIndex(t *testing.T, files []spackbundle.IndexFile) []byte {
	t.Helper()
	index := spackbundle.Index{
		APIVersion: spackbundle.FormatVersion,
		Kind:       "BundleIndex",
		CreatedAt:  time.Unix(1, 0).UTC(),
		Files:      files,
	}
	payload, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("SPACKIDX\x00")
	body = binary.BigEndian.AppendUint32(body, boundaryUint32(t, len(payload)))
	body = append(body, payload...)
	return body
}

func boundaryUint32(t *testing.T, value int) uint32 {
	t.Helper()
	parsed, err := strconv.ParseUint(strconv.Itoa(value), 10, 32)
	if err != nil {
		t.Fatalf("value %d exceeds uint32 range: %v", value, err)
	}
	return uint32(parsed)
}

func boundaryIndexFile(path string, body []byte) spackbundle.IndexFile {
	digest := sha256.Sum256(body)
	return spackbundle.IndexFile{
		Path:   path,
		Kind:   "asset",
		Size:   int64(len(body)),
		SHA256: hex.EncodeToString(digest[:]),
	}
}
