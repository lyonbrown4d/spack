package catalog_test

import (
	"testing"

	"github.com/lyonbrown4d/spack/internal/catalog"
)

func TestDeleteVariantsRemovesAssetVariantsOnly(t *testing.T) {
	cat := catalog.NewInMemoryCatalog()

	for _, asset := range []*catalog.Asset{
		{
			Path:       "app.js",
			FullPath:   "/assets/app.js",
			MediaType:  "application/javascript",
			SourceHash: "hash-1",
			ETag:       "\"hash-1\"",
		},
		{
			Path:       "style.css",
			FullPath:   "/assets/style.css",
			MediaType:  "text/css",
			SourceHash: "hash-2",
			ETag:       "\"hash-2\"",
		},
	} {
		if err := cat.UpsertAsset(asset); err != nil {
			t.Fatal(err)
		}
	}

	for _, variant := range []*catalog.Variant{
		{
			ID:           "app.js|encoding=br",
			AssetPath:    "app.js",
			ArtifactPath: "/artifacts/app.js.br",
			MediaType:    "application/javascript",
			SourceHash:   "hash-1",
			ETag:         "\"hash-1-br\"",
			Encoding:     "br",
		},
		{
			ID:           "style.css|encoding=gzip",
			AssetPath:    "style.css",
			ArtifactPath: "/artifacts/style.css.gz",
			MediaType:    "text/css",
			SourceHash:   "hash-2",
			ETag:         "\"hash-2-gzip\"",
			Encoding:     "gzip",
		},
	} {
		if err := cat.UpsertVariant(variant); err != nil {
			t.Fatal(err)
		}
	}

	removed := cat.DeleteVariants("app.js")
	if removed.Len() != 1 {
		t.Fatalf("expected one removed variant, got %d", removed.Len())
	}
	if cat.ListVariants("app.js").Len() != 0 {
		t.Fatalf("expected app.js variants removed, got %#v", cat.ListVariants("app.js"))
	}
	if cat.ListVariants("style.css").Len() != 1 {
		t.Fatalf("expected style.css variants to remain, got %#v", cat.ListVariants("style.css"))
	}
}

func TestDeleteAssetRemovesAssetAndVariants(t *testing.T) {
	cat := catalog.NewInMemoryCatalog()
	asset := &catalog.Asset{
		Path:       "app.js",
		FullPath:   "/assets/app.js",
		MediaType:  "application/javascript",
		SourceHash: "hash-1",
		ETag:       "\"hash-1\"",
	}
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}
	if err := cat.UpsertVariant(&catalog.Variant{
		ID:           "app.js|encoding=br",
		AssetPath:    "app.js",
		ArtifactPath: "/artifacts/app.js.br",
		MediaType:    "application/javascript",
		SourceHash:   "hash-1",
		ETag:         "\"hash-1-br\"",
		Encoding:     "br",
	}); err != nil {
		t.Fatal(err)
	}

	removed := cat.DeleteAsset("app.js")
	if removed.Len() != 1 {
		t.Fatalf("expected one removed variant, got %d", removed.Len())
	}
	if _, ok := cat.FindAsset("app.js"); ok {
		t.Fatal("expected asset to be deleted")
	}
	if cat.VariantCount() != 0 {
		t.Fatalf("expected no variants after delete asset, got %d", cat.VariantCount())
	}
}
