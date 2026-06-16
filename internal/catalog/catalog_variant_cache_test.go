package catalog_test

import (
	"slices"
	"testing"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/catalog"
)

func TestVariantListCachesInvalidateOnWrites(t *testing.T) {
	cat := catalog.NewInMemoryCatalog()
	upsertVariantCacheAssets(t, cat)
	upsertVariantCacheInitialVariant(t, cat)
	warmVariantListCaches(cat)

	upsertVariantCacheAdditionalVariants(t, cat)
	assertVariantCacheAfterInsert(t, cat)

	if !cat.DeleteVariantByArtifactPath("/artifacts/app.js.gz") {
		t.Fatal("expected gzip variant to be deleted")
	}
	assertVariantCacheAfterDelete(t, cat)
}

func upsertVariantCacheAssets(t *testing.T, cat catalog.Catalog) {
	t.Helper()
	for _, asset := range []*catalog.Asset{
		{
			Path:       "app.js",
			FullPath:   "/assets/app.js",
			MediaType:  "application/javascript",
			SourceHash: "hash-1",
			ETag:       "\"hash-1\"",
		},
		{
			Path:       "hero.jpg",
			FullPath:   "/assets/hero.jpg",
			MediaType:  "image/jpeg",
			SourceHash: "hash-2",
			ETag:       "\"hash-2\"",
		},
	} {
		if err := cat.UpsertAsset(asset); err != nil {
			t.Fatal(err)
		}
	}
}

func upsertVariantCacheInitialVariant(t *testing.T, cat catalog.Catalog) {
	t.Helper()
	if err := cat.UpsertVariant(&catalog.Variant{
		ID:           "app.js|encoding=br",
		AssetPath:    "app.js",
		ArtifactPath: "/artifacts/app.js.br",
		MediaType:    "application/javascript",
		SourceHash:   "hash-1",
		ETag:         "\"hash-1-br\"",
		Encoding:     "br",
		Metadata: cxmapping.NewMapFrom(map[string]string{
			"stage": "compression",
		}),
	}); err != nil {
		t.Fatal(err)
	}
}

func warmVariantListCaches(cat catalog.Catalog) {
	_ = cat.ListVariants("app.js")
	_ = cat.ListVariantsByStage("compression")
	_ = cat.AllVariants()
	_ = cat.ListImageVariants("hero.jpg", "jpeg")
}

func upsertVariantCacheAdditionalVariants(t *testing.T, cat catalog.Catalog) {
	t.Helper()
	for _, variant := range []*catalog.Variant{
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
		{
			ID:           "hero.jpg|width=640",
			AssetPath:    "hero.jpg",
			ArtifactPath: "/artifacts/hero.w640.jpg",
			MediaType:    "image/jpeg",
			SourceHash:   "hash-2",
			ETag:         "\"hash-2-w640\"",
			Format:       "jpeg",
			Width:        640,
		},
	} {
		if err := cat.UpsertVariant(variant); err != nil {
			t.Fatal(err)
		}
	}
}

func assertVariantCacheAfterInsert(t *testing.T, cat catalog.Catalog) {
	t.Helper()
	wantAppIDs := []string{"app.js|encoding=br", "app.js|encoding=gzip"}
	assertVariantIDs(t, cat.ListVariants("app.js"), wantAppIDs)
	assertVariantIDs(t, cat.ListVariantsByStage("compression"), wantAppIDs)
	assertVariantCount(t, cat.ListImageVariants("hero.jpg", "jpeg"), 1, "image variant cache")
	assertVariantCount(t, cat.AllVariants(), 3, "all variant cache")
}

func assertVariantCacheAfterDelete(t *testing.T, cat catalog.Catalog) {
	t.Helper()
	wantAppIDs := []string{"app.js|encoding=br"}
	assertVariantIDs(t, cat.ListVariants("app.js"), wantAppIDs)
	assertVariantIDs(t, cat.ListVariantsByStage("compression"), wantAppIDs)
	assertVariantCount(t, cat.AllVariants(), 2, "all variant cache")
}

func assertVariantIDs(t *testing.T, variants *cxlist.List[*catalog.Variant], want []string) {
	t.Helper()
	if got := variantIDs(variants); !slices.Equal(got, want) {
		t.Fatalf("expected variant ids %v, got %v", want, got)
	}
}

func assertVariantCount(t *testing.T, variants *cxlist.List[*catalog.Variant], want int, label string) {
	t.Helper()
	if got := variants.Len(); got != want {
		t.Fatalf("expected %s count %d, got %#v", label, want, variants.Values())
	}
}

func variantIDs(variants *cxlist.List[*catalog.Variant]) []string {
	ids := make([]string, 0, variants.Len())
	variants.Range(func(_ int, variant *catalog.Variant) bool {
		ids = append(ids, variant.ID)
		return true
	})
	return ids
}
