package spackbundle

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestBundleIndexCapacityCoversOneMillionRepresentativeAOTFiles(t *testing.T) {
	t.Parallel()

	representative := IndexFile{
		Path:       "assets/images/landing/hero-background-DD5pHGF0.webp",
		Kind:       "image_variant",
		Size:       684_354,
		SHA256:     strings.Repeat("a", sha256.Size*2),
		MediaType:  "image/webp",
		SourceHash: strings.Repeat("b", sha256.Size*2),
		ETag:       `"sha256-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`,
		AssetPath:  "assets/images/landing/hero-background-DD5pHGF0.webp",
		Encoding:   "identity",
		Format:     "webp",
		Width:      1920,
	}
	entry, err := json.Marshal(representative)
	if err != nil {
		t.Fatal(err)
	}
	emptyIndex, err := json.Marshal(Index{
		APIVersion: FormatVersion,
		Kind:       "BundleIndex",
		CreatedAt:  time.Unix(1, 0).UTC(),
		Files:      []IndexFile{},
	})
	if err != nil {
		t.Fatal(err)
	}

	const capacityHeadroomPercent = 25
	projectedPayloadBytes := int64(len(emptyIndex)) + int64(maxBundleFiles)*(int64(len(entry))+1)
	requiredBytes := projectedPayloadBytes + projectedPayloadBytes*capacityHeadroomPercent/100
	maxPayloadBytes := maxBundleIndexBytes - int64(len(indexMagic)) - 4
	if requiredBytes > maxPayloadBytes {
		t.Fatalf(
			"representative entry is %d bytes; one million entries plus %d%% headroom require %d bytes, limit allows %d",
			len(entry), capacityHeadroomPercent, requiredBytes, maxPayloadBytes,
		)
	}
	if maxBundleIndexBytes > math.MaxUint32 {
		t.Fatalf("bundle index limit %d exceeds uint32 format bound", maxBundleIndexBytes)
	}
}

func TestReadBundleIndexRejectsOversizedEntry(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	writer := tar.NewWriter(&body)
	err := writer.WriteHeader(&tar.Header{
		Name:     IndexPath,
		Mode:     0o600,
		Size:     maxBundleIndexBytes + 1,
		Typeflag: tar.TypeReg,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = readBundleIndex(tar.NewReader(&body), productionBundleLimits)
	if err == nil {
		t.Fatal("expected oversized index entry to be rejected")
	}
	if !strings.Contains(err.Error(), "bundle index exceeds max bytes") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnmarshalIndexRejectsOversizedDeclaredPayload(t *testing.T) {
	t.Parallel()

	body := bytes.Clone(indexMagic)
	body = binary.BigEndian.AppendUint32(body, math.MaxUint32)

	_, err := unmarshalIndex(body, productionBundleLimits)
	if err == nil {
		t.Fatal("expected oversized declared index payload to be rejected")
	}
	if !strings.Contains(err.Error(), "bundle index exceeds max bytes") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBundleFileCountBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		count   int
		wantErr bool
	}{
		{name: "maximum accepted", count: maxBundleFiles},
		{name: "over maximum rejected", count: maxBundleFiles + 1, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateBundleFileCount(tt.count, productionBundleLimits)
			if tt.wantErr && err == nil {
				t.Fatal("expected file count to be rejected")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected file count to be accepted: %v", err)
			}
		})
	}
}

func TestValidateIndexExpandedBytesBoundaries(t *testing.T) {
	t.Parallel()

	files := declaredFilesAtExpandedLimit()
	if err := validateIndex(Index{Files: files}, productionBundleLimits); err != nil {
		t.Fatalf("expected exact expanded byte limit to be accepted: %v", err)
	}

	overLimit := append([]IndexFile(nil), files...)
	overLimit = append(overLimit, declaredIndexFile("assets/overflow.bin", 1))
	err := validateIndex(Index{Files: overLimit}, productionBundleLimits)
	assertExpandedLimitError(t, err)
}

func TestBundleBudgetExpandedBytesBoundariesPreserveState(t *testing.T) {
	t.Parallel()

	budget := bundleBudget{
		limits:        productionBundleLimits,
		files:         1,
		expandedBytes: maxExpandedBundleBytes - 1,
	}
	if err := budget.add(1); err != nil {
		t.Fatalf("expected exact expanded byte limit to be accepted: %v", err)
	}
	if budget.expandedBytes != maxExpandedBundleBytes || budget.files != 2 {
		t.Fatalf("unexpected budget at limit: %+v", budget)
	}

	atLimit := budget
	err := budget.add(1)
	if err == nil {
		t.Fatal("expected expanded byte limit + 1 to be rejected")
	}
	if budget != atLimit {
		t.Fatalf("failed add changed budget: before=%+v after=%+v", atLimit, budget)
	}

	budget = bundleBudget{limits: productionBundleLimits}
	empty := budget
	err = budget.add(math.MaxInt64)
	if err == nil {
		t.Fatal("expected math.MaxInt64 to be rejected")
	}
	if budget != empty {
		t.Fatalf("failed max-int add changed budget: before=%+v after=%+v", empty, budget)
	}
}

func TestBundleBudgetRejectsStreamingFileOverflow(t *testing.T) {
	t.Parallel()

	budget := bundleBudget{limits: productionBundleLimits, files: maxBundleFiles}
	err := budget.add(0)
	if err == nil {
		t.Fatal("expected streaming file count to be rejected")
	}
	if !strings.Contains(err.Error(), "bundle exceeds max file count") {
		t.Fatalf("unexpected error: %v", err)
	}
}
