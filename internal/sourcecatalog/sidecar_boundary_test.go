package sourcecatalog_test

import (
	"path/filepath"
	"testing"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
)

func TestScannerSidecarTrustBoundaries(t *testing.T) {
	root := t.TempDir()
	png := validPNGForSidecarTest(t)
	writeSourceFile(t, filepath.Join(root, "hero.png"), png)
	writeCompressedSourceFile(t, filepath.Join(root, "hero.png.gz"), "gzip", png)
	writeSourceFile(t, filepath.Join(root, "bad.png"), png)
	writeCompressedSourceFile(t, filepath.Join(root, "bad.png.gz"), "gzip", []byte("not a png"))
	writeSourceFile(t, filepath.Join(root, "app.js"), []byte("console.log('ok')"))
	writeCompressedSourceFile(t, filepath.Join(root, "app.js.gz"), "gzip", []byte("console.log('ok')"))
	writeSourceFile(t, filepath.Join(root, "broken.js"), []byte("console.log('broken')"))
	writeSourceFile(t, filepath.Join(root, "broken.js.gz"), []byte{0x1f, 0x8b})

	scanner := newScannerForTest(t, root, cxlist.NewList("gzip"))
	snapshot, err := scanner.Scan(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	assertVariantPresentForBoundaryTest(t, snapshot, "hero.png.gz")
	assertVariantPresentForBoundaryTest(t, snapshot, "app.js.gz")
	assertVariantMissingForBoundaryTest(t, snapshot, "bad.png.gz")
	assertVariantMissingForBoundaryTest(t, snapshot, "broken.js.gz")
}

func TestScannerKeepsOrphanSidecarAsPlainAsset(t *testing.T) {
	root := t.TempDir()
	writeCompressedSourceFile(t, filepath.Join(root, "orphan.png.gz"), "gzip", validPNGForSidecarTest(t))

	scanner := newScannerForTest(t, root, cxlist.NewList("gzip"))
	snapshot, err := scanner.Scan(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	assertVariantMissingForBoundaryTest(t, snapshot, "orphan.png.gz")
	if _, ok := snapshot.Assets.Get("orphan.png.gz"); !ok {
		t.Fatal("expected sidecar-looking orphan to remain a plain asset")
	}
}

func assertVariantPresentForBoundaryTest(t *testing.T, snapshot sourcecatalog.Snapshot, id string) {
	t.Helper()
	if variant, ok := snapshot.Variants.Get(id); !ok || variant == nil {
		t.Fatalf("expected variant %s to be present", id)
	}
}

func assertVariantMissingForBoundaryTest(t *testing.T, snapshot sourcecatalog.Snapshot, id string) {
	t.Helper()
	if variant, ok := snapshot.Variants.Get(id); ok || variant != nil {
		t.Fatalf("expected variant %s to be absent", id)
	}
}
