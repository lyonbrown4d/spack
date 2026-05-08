package task_test

import (
	"context"
	"github.com/daiyuang/spack/internal/catalog"
	"github.com/daiyuang/spack/internal/source"
	"github.com/daiyuang/spack/internal/sourcecatalog"
	"github.com/daiyuang/spack/internal/task"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncSourceCatalogRemovesDeletedAssetsAndVariants(t *testing.T) {
	root := t.TempDir()
	src := newLocalSourceForTest(t, root)
	cat := catalog.NewInMemoryCatalog()
	artifactPath := filepath.Join(root, "cache", "app.js.br")

	upsertAssetForTest(t, cat, &catalog.Asset{
		Path:       "app.js",
		FullPath:   filepath.Join(root, "app.js"),
		MediaType:  "application/javascript",
		SourceHash: "hash-old",
		ETag:       "\"hash-old\"",
	})
	upsertVariantForTest(t, cat, artifactPath)

	report, err := task.SyncSourceCatalogForTest(context.Background(), src, cat, nil)
	if err != nil {
		t.Fatal(err)
	}

	assertOne(t, report.Removed, "removed asset")
	assertOne(t, report.RemovedVariants, "removed variant")
	if _, ok := cat.FindAsset("app.js"); ok {
		t.Fatal("expected app.js to be removed from catalog")
	}
	assertFileRemoved(t, artifactPath)
}

func TestSyncSourceCatalogRemovesVariantsForChangedAsset(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "app.js")
	writeFileForTest(t, assetPath, []byte("console.log('new');"))

	src := newLocalSourceForTest(t, root)
	cat := catalog.NewInMemoryCatalog()
	artifactPath := filepath.Join(root, "cache", "app.js.br")

	upsertAssetForTest(t, cat, &catalog.Asset{
		Path:       "app.js",
		FullPath:   assetPath,
		Size:       3,
		MediaType:  "application/javascript",
		SourceHash: "hash-old",
		ETag:       "\"hash-old\"",
	})
	upsertVariantForTest(t, cat, artifactPath)

	report, err := task.SyncSourceCatalogForTest(context.Background(), src, cat, nil)
	if err != nil {
		t.Fatal(err)
	}

	assertOne(t, report.Updated, "updated asset")
	assertOne(t, report.RemovedVariants, "removed variant")
	if cat.ListVariants("app.js").Len() != 0 {
		t.Fatalf("expected app.js variants to be removed, got %#v", cat.ListVariants("app.js"))
	}

	asset, ok := cat.FindAsset("app.js")
	if !ok {
		t.Fatal("expected app.js to remain in catalog")
	}
	if asset.SourceHash == "hash-old" {
		t.Fatal("expected app.js source hash to be refreshed")
	}
	assertFileRemoved(t, artifactPath)
}

func TestSyncSourceCatalogRecognizesSourceCompressionSidecars(t *testing.T) {
	root := t.TempDir()
	writeFileForTest(t, filepath.Join(root, "app.js"), []byte("console.log('new');"))
	sidecarPath := filepath.Join(root, "app.js.br")
	writeFileForTest(t, sidecarPath, []byte("compressed"))

	src := newLocalSourceForTest(t, root)
	cat := catalog.NewInMemoryCatalog()

	report, err := task.SyncSourceCatalogForTest(context.Background(), src, cat, nil)
	if err != nil {
		t.Fatal(err)
	}

	if report.Added != 1 {
		t.Fatalf("expected one added asset, got %d", report.Added)
	}
	if _, ok := cat.FindAsset("app.js"); !ok {
		t.Fatal("expected app.js asset to be present")
	}
	if _, ok := cat.FindAsset("app.js.br"); ok {
		t.Fatal("expected app.js.br not to be registered as plain asset")
	}

	variants := cat.ListVariants("app.js")
	if variants.Len() != 1 {
		t.Fatalf("expected one source sidecar variant, got %d", variants.Len())
	}
	variant := singleVariantForTest(t, variants)
	if variant.ArtifactPath != sidecarPath {
		t.Fatalf("expected sidecar artifact path %q, got %q", sidecarPath, variant.ArtifactPath)
	}
	if variant.Encoding != "br" {
		t.Fatalf("expected br encoding, got %q", variant.Encoding)
	}
	if !sourcecatalog.IsSourceSidecarVariant(variant) {
		t.Fatal("expected source sidecar metadata marker")
	}
}

func TestSyncSourceCatalogRemovesMissingSourceSidecarVariant(t *testing.T) {
	root := t.TempDir()
	writeFileForTest(t, filepath.Join(root, "app.js"), []byte("console.log('new');"))
	sidecarPath := filepath.Join(root, "app.js.br")
	writeFileForTest(t, sidecarPath, []byte("compressed"))

	src := newLocalSourceForTest(t, root)
	cat := catalog.NewInMemoryCatalog()

	if _, err := task.SyncSourceCatalogForTest(context.Background(), src, cat, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sidecarPath); err != nil {
		t.Fatal(err)
	}

	report, err := task.SyncSourceCatalogForTest(context.Background(), src, cat, nil)
	if err != nil {
		t.Fatal(err)
	}

	assertOne(t, report.RemovedVariants, "removed variant")
	if report.RemovedArtifacts != 0 {
		t.Fatalf("expected no artifact deletions for missing source sidecar, got %d", report.RemovedArtifacts)
	}
	if cat.ListVariants("app.js").Len() != 0 {
		t.Fatalf("expected no variants after removing source sidecar, got %#v", cat.ListVariants("app.js"))
	}
	if _, ok := cat.FindAsset("app.js"); !ok {
		t.Fatal("expected app.js asset to remain in catalog")
	}
}

func TestSyncSourceCatalogKeepsSourceSidecarFileOnAssetRefresh(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "app.js")
	sidecarPath := filepath.Join(root, "app.js.br")
	writeFileForTest(t, assetPath, []byte("console.log('old');"))
	writeFileForTest(t, sidecarPath, []byte("compressed"))

	src := newLocalSourceForTest(t, root)
	cat := catalog.NewInMemoryCatalog()

	if _, err := task.SyncSourceCatalogForTest(context.Background(), src, cat, nil); err != nil {
		t.Fatal(err)
	}
	originalVariant := singleVariantForTest(t, cat.ListVariants("app.js"))
	writeFileForTest(t, assetPath, []byte("console.log('new-content');"))

	report, err := task.SyncSourceCatalogForTest(context.Background(), src, cat, nil)
	if err != nil {
		t.Fatal(err)
	}

	assertOne(t, report.Updated, "updated asset")
	assertOne(t, report.RemovedVariants, "removed variant")
	if report.RemovedArtifacts != 0 {
		t.Fatalf("expected no artifact deletions for source sidecars, got %d", report.RemovedArtifacts)
	}
	if _, err := os.Stat(sidecarPath); err != nil {
		t.Fatalf("expected source sidecar file to remain on disk, got err=%v", err)
	}

	variants := cat.ListVariants("app.js")
	if variants.Len() != 1 {
		t.Fatalf("expected sidecar variant to be re-added, got %d", variants.Len())
	}
	refreshedVariant := singleVariantForTest(t, variants)
	if refreshedVariant.SourceHash == originalVariant.SourceHash {
		t.Fatal("expected source sidecar variant to refresh with new asset source hash")
	}
}

func TestSyncSourceCatalogIncrementallyUpdatesAssetAndRebuildsSidecars(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "app.js")
	writeFileForTest(t, assetPath, []byte("console.log('old');"))

	src := newLocalSourceForTest(t, root)
	cat := catalog.NewInMemoryCatalog()

	if _, err := task.SyncSourceCatalogForTest(context.Background(), src, cat, nil); err != nil {
		t.Fatal(err)
	}

	writeFileForTest(t, assetPath, []byte("console.log('new-content');"))
	report, err := task.SyncSourceCatalogForTest(context.Background(), src, cat, nil, source.ChangeEvent{
		Path: "app.js",
		Op:   "WRITE",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Updated != 1 {
		t.Fatalf("expected 1 updated asset, got %d", report.Updated)
	}
	if report.Scanned != 1 {
		t.Fatalf("expected scanned count 1, got %d", report.Scanned)
	}
}

func TestSyncSourceCatalogIncrementallyRemovesSidecarVariant(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "app.js")
	sidecarPath := filepath.Join(root, "app.js.br")
	writeFileForTest(t, assetPath, []byte("console.log('old');"))
	writeFileForTest(t, sidecarPath, []byte("compressed"))

	src := newLocalSourceForTest(t, root)
	cat := catalog.NewInMemoryCatalog()

	if _, err := task.SyncSourceCatalogForTest(context.Background(), src, cat, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sidecarPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sidecarPath); err != nil {
		t.Fatal(err)
	}

	report, err := task.SyncSourceCatalogForTest(context.Background(), src, cat, nil, source.ChangeEvent{
		Path: "app.js.br",
		Op:   "REMOVE",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.RemovedVariants != 1 {
		t.Fatalf("expected 1 removed variant, got %d", report.RemovedVariants)
	}
	if cat.ListVariants("app.js").Len() != 0 {
		t.Fatalf("expected no variants after removing sidecar, got %#v", cat.ListVariants("app.js"))
	}
}

func TestSyncSourceCatalogIncrementallyFallsBackToFullOnRename(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "app.js")
	writeFileForTest(t, assetPath, []byte("console.log('old');"))

	src := newLocalSourceForTest(t, root)
	cat := catalog.NewInMemoryCatalog()

	if _, err := task.SyncSourceCatalogForTest(context.Background(), src, cat, nil); err != nil {
		t.Fatal(err)
	}
	report, err := task.SyncSourceCatalogForTest(context.Background(), src, cat, nil, source.ChangeEvent{
		Path: "app.js",
		Op:   "RENAME",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Scanned != 1 {
		t.Fatalf("expected fallback full scan with scanned count 1, got %d", report.Scanned)
	}
}
