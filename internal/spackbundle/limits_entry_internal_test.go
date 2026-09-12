package spackbundle

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type writeLimitTestCase struct {
	name    string
	bodies  [][]byte
	limits  bundleLimits
	wantErr string
}

func TestWriteWithLimitsEnforcesExecutionPath(t *testing.T) {
	t.Parallel()

	tests := []writeLimitTestCase{
		{
			name:    "file count",
			bodies:  [][]byte{[]byte("a"), []byte("b")},
			limits:  limitTestLimits(1, 10, 10, 1<<20),
			wantErr: "bundle exceeds max file count",
		},
		{
			name:    "expanded bytes",
			bodies:  [][]byte{[]byte("ab")},
			limits:  limitTestLimits(1, 1, 10, 1<<20),
			wantErr: "bundle exceeds max expanded bytes",
		},
		{
			name:    "final index",
			bodies:  [][]byte{[]byte("a")},
			limits:  limitTestLimits(1, 10, 10, 32),
			wantErr: "bundle index exceeds max bytes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertWriteLimit(t, tt)
		})
	}
}

func assertWriteLimit(t *testing.T, tt writeLimitTestCase) {
	t.Helper()

	root := t.TempDir()
	files := make([]File, 0, len(tt.bodies))
	for index, body := range tt.bodies {
		name := fmt.Sprintf("asset-%d.bin", index)
		fullPath := filepath.Join(root, name)
		if err := os.WriteFile(fullPath, body, 0o600); err != nil {
			t.Fatal(err)
		}
		files = append(files, File{
			Path:     name,
			FullPath: fullPath,
			Kind:     "asset",
		})
	}
	output := filepath.Join(t.TempDir(), "output.spack")
	_, err := writeWithLimits(t.Context(), WriteOptions{
		Output: output,
		Root:   root,
		Files:  files,
	}, tt.limits)
	assertLimitError(t, err, tt.wantErr)
	if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
		t.Fatalf("expected failed write not to publish output, got %v", statErr)
	}
}

func TestVerifyAndExtractEnforcePayloadStreamingLimits(t *testing.T) {
	t.Parallel()

	body := []byte("a")
	file := limitIndexFile("asset.bin", body)
	entries := []limitBundleEntry{
		{path: file.Path, body: body},
		{path: file.Path, body: body},
	}
	tests := []struct {
		name    string
		limits  bundleLimits
		wantErr string
	}{
		{
			name:    "file count",
			limits:  limitTestLimits(1, 10, 10, 1<<20),
			wantErr: "bundle exceeds max file count",
		},
		{
			name:    "expanded bytes",
			limits:  limitTestLimits(2, 1, 10, 1<<20),
			wantErr: "bundle exceeds max expanded bytes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertStreamingLimit(t, file, entries, tt.limits, tt.wantErr)
		})
	}
}

func assertStreamingLimit(
	t *testing.T,
	file IndexFile,
	entries []limitBundleEntry,
	limits bundleLimits,
	wantErr string,
) {
	t.Helper()

	bundlePath := writeLimitBundle(t, []IndexFile{file}, entries, limits)
	if _, err := readIndexWithLimits(bundlePath, limits); err != nil {
		t.Fatalf("expected index to remain valid before streaming payload: %v", err)
	}
	_, err := verifyBundleWithLimits(t.Context(), bundlePath, limits)
	assertLimitError(t, err, wantErr)
	_, err = extractToWithLimits(
		t.Context(),
		bundlePath,
		filepath.Join(t.TempDir(), "extract"),
		limits,
	)
	assertLimitError(t, err, wantErr)
}

func TestReaderReadFileEnforcesSingleFileLimit(t *testing.T) {
	t.Parallel()

	declaredBody := []byte("a")
	file := limitIndexFile("asset.bin", declaredBody)
	limits := limitTestLimits(1, 10, 1, 1<<20)
	bundlePath := writeLimitBundle(
		t,
		[]IndexFile{file},
		[]limitBundleEntry{{path: file.Path, body: []byte("ab")}},
		limits,
	)

	reader, err := openReaderWithLimits(bundlePath, limits)
	if err != nil {
		t.Fatal(err)
	}
	if _, indexErr := reader.Index(); indexErr != nil {
		t.Fatalf("expected index to be valid before reading payload: %v", indexErr)
	}
	_, err = reader.ReadFile(file.Path)
	assertLimitError(t, err, "exceeds max extracted bytes")
}

func TestMarshalIndexWithLimitsAcceptsExactBoundary(t *testing.T) {
	t.Parallel()

	index := Index{
		CreatedAt: time.Unix(1, 0).UTC(),
		Files:     []IndexFile{limitIndexFile("asset.bin", []byte("a"))},
	}
	body, err := marshalIndex(index, productionBundleLimits)
	if err != nil {
		t.Fatal(err)
	}

	exact := productionBundleLimits
	exact.indexBytes = int64(len(body))
	if _, marshalErr := marshalIndex(index, exact); marshalErr != nil {
		t.Fatalf("expected exact index limit to be accepted: %v", marshalErr)
	}

	under := exact
	under.indexBytes--
	_, err = marshalIndex(index, under)
	assertLimitError(t, err, "bundle index exceeds max bytes")
}
