package sourcecatalog_test

import (
	"context"
	"path/filepath"
	"testing"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
)

type sidecarEncodingBoundaryCase struct {
	encoding string
	suffix   string
}

func TestScannerTrustsValidExternalSidecarsForAllEnabledEncodings(t *testing.T) {
	root := t.TempDir()
	cases := sidecarEncodingBoundaryCases()
	writeSidecarEncodingFixtures(t, root, cases, validPNGForSidecarTest(t), validPNGForSidecarTest(t), "valid")

	snapshot := scanSidecarEncodingBoundary(t, root)

	for _, tc := range cases {
		t.Run(tc.encoding, func(t *testing.T) {
			assertTrustedSidecarEncoding(t, snapshot, "valid-"+tc.encoding+".png", tc)
		})
	}
}

func TestScannerRejectsMismatchedImageMagicForAllEnabledEncodings(t *testing.T) {
	root := t.TempDir()
	cases := sidecarEncodingBoundaryCases()
	writeSidecarEncodingFixtures(t, root, cases, validPNGForSidecarTest(t), []byte("not a png"), "invalid")

	snapshot := scanSidecarEncodingBoundary(t, root)

	for _, tc := range cases {
		t.Run(tc.encoding, func(t *testing.T) {
			assertRejectedSidecarEncoding(t, snapshot, "invalid-"+tc.encoding+".png", tc)
		})
	}
}

func writeSidecarEncodingFixtures(
	t *testing.T,
	root string,
	cases []sidecarEncodingBoundaryCase,
	assetBody []byte,
	sidecarBody []byte,
	prefix string,
) {
	t.Helper()
	for _, tc := range cases {
		asset := prefix + "-" + tc.encoding + ".png"
		writeSourceFile(t, filepath.Join(root, asset), assetBody)
		writeCompressedSourceFile(t, filepath.Join(root, asset+tc.suffix), tc.encoding, sidecarBody)
	}
}

func scanSidecarEncodingBoundary(t *testing.T, root string) sourcecatalog.Snapshot {
	t.Helper()
	scanner := newScannerForTest(t, root, cxlist.NewList("gzip", "br", "zstd"))
	snapshot, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func assertTrustedSidecarEncoding(
	t *testing.T,
	snapshot sourcecatalog.Snapshot,
	asset string,
	tc sidecarEncodingBoundaryCase,
) {
	t.Helper()
	variantID := asset + tc.suffix
	variant, ok := snapshot.Variants.Get(variantID)
	if !ok || variant == nil {
		t.Fatalf("expected valid %s sidecar to be registered", tc.encoding)
	}
	if variant.Encoding != tc.encoding {
		t.Fatalf("expected encoding %q, got %q", tc.encoding, variant.Encoding)
	}
	assertSidecarHiddenFromAssets(t, snapshot, variantID)
}

func assertRejectedSidecarEncoding(
	t *testing.T,
	snapshot sourcecatalog.Snapshot,
	asset string,
	tc sidecarEncodingBoundaryCase,
) {
	t.Helper()
	variantID := asset + tc.suffix
	if _, ok := snapshot.Variants.Get(variantID); ok {
		t.Fatalf("expected mismatched %s sidecar to be skipped", tc.encoding)
	}
	assertSidecarHiddenFromAssets(t, snapshot, variantID)
}

func assertSidecarHiddenFromAssets(t *testing.T, snapshot sourcecatalog.Snapshot, variantID string) {
	t.Helper()
	if _, ok := snapshot.Assets.Get(variantID); ok {
		t.Fatalf("expected %s sidecar to be hidden from plain assets", variantID)
	}
}

func sidecarEncodingBoundaryCases() []sidecarEncodingBoundaryCase {
	return []sidecarEncodingBoundaryCase{
		{encoding: "gzip", suffix: ".gz"},
		{encoding: "br", suffix: ".br"},
		{encoding: "zstd", suffix: ".zst"},
	}
}
