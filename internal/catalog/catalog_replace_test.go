package catalog_test

import (
	"testing"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/catalog"
)

func TestReplaceCatalogPublishesBulkSnapshot(t *testing.T) {
	cat := catalog.NewInMemoryCatalog()
	err := cat.ReplaceCatalog(catalog.ReplaceCatalogInput{
		Assets: cxlist.NewList(&catalog.Asset{
			Path:       "app.js",
			FullPath:   "/assets/app.js",
			MediaType:  "application/javascript",
			SourceHash: "hash-1",
			ETag:       "\"hash-1\"",
		}),
		Variants: cxlist.NewList(&catalog.Variant{
			AssetPath:    "app.js",
			ArtifactPath: "/artifacts/app.js.br",
			MediaType:    "application/javascript",
			SourceHash:   "hash-1",
			ETag:         "\"hash-1-br\"",
			Encoding:     "br",
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := cat.AssetCount(); got != 1 {
		t.Fatalf("expected one asset, got %d", got)
	}
	if got := cat.VariantCount(); got != 1 {
		t.Fatalf("expected one variant, got %d", got)
	}
	variant, ok := cat.FindEncodingVariant("app.js", "br")
	if !ok {
		t.Fatal("expected bulk-loaded encoding variant")
	}
	if variant.ID != "app.js|encoding=br" {
		t.Fatalf("expected default variant id, got %q", variant.ID)
	}

	if err := cat.ReplaceCatalog(catalog.ReplaceCatalogInput{
		Assets: cxlist.NewList(&catalog.Asset{
			Path:       "style.css",
			FullPath:   "/assets/style.css",
			MediaType:  "text/css",
			SourceHash: "hash-2",
			ETag:       "\"hash-2\"",
		}),
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := cat.FindAsset("app.js"); ok {
		t.Fatal("expected old asset to be replaced")
	}
	if got := cat.VariantCount(); got != 0 {
		t.Fatalf("expected old variants to be replaced, got %d", got)
	}
}

func TestReplaceCatalogRejectsMissingVariantAsset(t *testing.T) {
	cat := catalog.NewInMemoryCatalog()
	err := cat.ReplaceCatalog(catalog.ReplaceCatalogInput{
		Variants: cxlist.NewList(&catalog.Variant{
			AssetPath:    "missing.js",
			ArtifactPath: "/artifacts/missing.js.br",
			Encoding:     "br",
		}),
	})
	if err == nil {
		t.Fatal("expected missing asset error")
	}
	if cat.AssetCount() != 0 || cat.VariantCount() != 0 {
		t.Fatal("expected failed replace to leave catalog unchanged")
	}
}
