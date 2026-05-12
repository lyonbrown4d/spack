package catalog_test

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"testing"
)

func TestFindEncodingVariantUsesAssetEncodingIndex(t *testing.T) {
	cat := catalog.NewInMemoryCatalog()

	if err := cat.UpsertAsset(&catalog.Asset{
		Path:       "app.js",
		FullPath:   "/assets/app.js",
		MediaType:  "application/javascript",
		SourceHash: "hash-1",
		ETag:       "\"hash-1\"",
	}); err != nil {
		t.Fatal(err)
	}
	if err := cat.UpsertVariant(&catalog.Variant{
		ID:           "custom-br-id",
		AssetPath:    "app.js",
		ArtifactPath: "/artifacts/app.js.br",
		MediaType:    "application/javascript",
		SourceHash:   "hash-1",
		ETag:         "\"hash-1-br\"",
		Encoding:     "br",
	}); err != nil {
		t.Fatal(err)
	}

	variant, ok := cat.FindEncodingVariant("app.js", "br")
	if !ok {
		t.Fatal("expected encoding variant to be present")
	}
	if variant.ID != "custom-br-id" {
		t.Fatalf("expected indexed variant id custom-br-id, got %q", variant.ID)
	}
}

func TestFindImageVariantFallsBackToMediaTypeWhenFormatEmpty(t *testing.T) {
	cat := catalog.NewInMemoryCatalog()

	if err := cat.UpsertAsset(&catalog.Asset{
		Path:       "hero.jpg",
		FullPath:   "/assets/hero.jpg",
		MediaType:  "image/jpeg",
		SourceHash: "hash-1",
		ETag:       "\"hash-1\"",
	}); err != nil {
		t.Fatal(err)
	}
	if err := cat.UpsertVariant(&catalog.Variant{
		ID:           "hero.jpg|width=640",
		AssetPath:    "hero.jpg",
		ArtifactPath: "/artifacts/hero.w640.jpg",
		MediaType:    "image/jpeg",
		SourceHash:   "hash-1",
		ETag:         "\"hash-1-w640\"",
		Width:        640,
	}); err != nil {
		t.Fatal(err)
	}

	variant, ok := cat.FindImageVariant("hero.jpg", "jpeg", 640)
	if !ok {
		t.Fatal("expected image variant lookup to use media type fallback")
	}
	if variant.Width != 640 {
		t.Fatalf("expected width 640 variant, got %#v", variant)
	}

	variants := cat.ListImageVariants("hero.jpg", "jpeg")
	if variants.Len() != 1 {
		t.Fatalf("expected image variant list to be filtered by derived format, got %#v", variants.Values())
	}
}

func TestListAndDeleteVariantsUseExactAssetPathIndex(t *testing.T) {
	cat := catalog.NewInMemoryCatalog()
	upsertIndexedAssetWithVariant(t, cat, "app")
	upsertIndexedAssetWithVariant(t, cat, "app.js")

	assertExactAppVariant(t, cat.ListVariants("app"))
	assertExactAppSnapshotVariant(t, cat.Snapshot())

	removed := cat.DeleteVariants("app")
	if removed.Len() != 1 {
		t.Fatalf("expected one app variant removed, got %d", removed.Len())
	}
	if got := cat.ListVariants("app.js"); got.Len() != 1 {
		t.Fatalf("expected app.js variants to remain, got %#v", got.Values())
	}
}

func TestListVariantsByStageAndAllVariants(t *testing.T) {
	cat := catalog.NewInMemoryCatalog()

	if err := cat.UpsertAsset(&catalog.Asset{
		Path:       "app.js",
		FullPath:   "/assets/app.js",
		MediaType:  "application/javascript",
		SourceHash: "hash-1",
		ETag:       "\"hash-1\"",
	}); err != nil {
		t.Fatal(err)
	}

	for _, variant := range []*catalog.Variant{
		{
			ID:           "app.js|encoding=br",
			AssetPath:    "app.js",
			ArtifactPath: "/assets/app.js.br",
			MediaType:    "application/javascript",
			SourceHash:   "hash-1",
			ETag:         "\"hash-1-br\"",
			Encoding:     "br",
			Metadata: cxmapping.NewMapFrom(map[string]string{
				"stage": "source_sidecar",
			}),
		},
		{
			ID:           "app.js|encoding=gzip",
			AssetPath:    "app.js",
			ArtifactPath: "/artifacts/app.js.gz",
			MediaType:    "application/javascript",
			SourceHash:   "hash-1",
			ETag:         "\"hash-1-gzip\"",
			Encoding:     "gzip",
			Metadata: cxmapping.NewMapFrom(map[string]string{
				"stage": "compression",
			}),
		},
	} {
		if err := cat.UpsertVariant(variant); err != nil {
			t.Fatal(err)
		}
	}

	sidecars := cat.ListVariantsByStage("source_sidecar")
	if sidecars.Len() != 1 {
		t.Fatalf("expected one source sidecar variant, got %#v", sidecars.Values())
	}
	sidecar, ok := sidecars.GetFirst()
	if !ok || sidecar.ID != "app.js|encoding=br" {
		t.Fatalf("expected br sidecar variant, got %#v", sidecar)
	}

	if all := cat.AllVariants(); all.Len() != 2 {
		t.Fatalf("expected all variants to return two entries, got %#v", all.Values())
	}
}

func upsertIndexedAssetWithVariant(t *testing.T, cat catalog.Catalog, assetPath string) {
	t.Helper()
	if err := cat.UpsertAsset(&catalog.Asset{
		Path:       assetPath,
		FullPath:   "/assets/" + assetPath,
		MediaType:  "application/javascript",
		SourceHash: "hash-" + assetPath,
		ETag:       "\"hash-" + assetPath + "\"",
	}); err != nil {
		t.Fatal(err)
	}
	if err := cat.UpsertVariant(&catalog.Variant{
		ID:           assetPath + "|encoding=br",
		AssetPath:    assetPath,
		ArtifactPath: "/artifacts/" + assetPath + ".br",
		MediaType:    "application/javascript",
		SourceHash:   "hash-" + assetPath,
		ETag:         "\"hash-" + assetPath + "-br\"",
		Encoding:     "br",
	}); err != nil {
		t.Fatal(err)
	}
}

func assertExactAppVariant(t *testing.T, variants *cxlist.List[*catalog.Variant]) {
	t.Helper()
	if variants.Len() != 1 {
		t.Fatalf("expected exact app variants only, got %#v", variants.Values())
	}
	first, ok := variants.GetFirst()
	if !ok || first.AssetPath != "app" {
		t.Fatalf("expected app variant, got %#v", first)
	}
}

func assertExactAppSnapshotVariant(t *testing.T, snapshot *catalog.Snapshot) {
	t.Helper()
	var snapshotEntry *catalog.Entry
	snapshot.Assets.Range(func(_ int, entry *catalog.Entry) bool {
		if entry.Asset != nil && entry.Asset.Path == "app" {
			snapshotEntry = entry
			return false
		}
		return true
	})
	if snapshotEntry == nil || snapshotEntry.Variants.Len() != 1 {
		t.Fatalf("expected exact app variants in snapshot, got %#v", snapshotEntry)
	}
}
