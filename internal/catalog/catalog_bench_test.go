package catalog_test

import (
	"fmt"
	"testing"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/catalog"
)

const (
	benchmarkCatalogAssetCount      = 10_000
	benchmarkCatalogVariantsPerPath = 4
)

func BenchmarkCatalogReplace(b *testing.B) {
	assets := benchmarkAssets(benchmarkCatalogAssetCount)
	variants := benchmarkVariants(benchmarkCatalogAssetCount, benchmarkCatalogVariantsPerPath)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		cat := catalog.NewInMemoryCatalog()
		if err := cat.ReplaceCatalog(catalog.ReplaceCatalogInput{Assets: assets, Variants: variants}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCatalogFindAsset(b *testing.B) {
	cat := benchmarkCatalog(b)
	paths := benchmarkAssetPaths()

	b.ReportAllocs()
	b.ResetTimer()
	i := 0
	for b.Loop() {
		if _, ok := cat.FindAsset(paths[i%len(paths)]); !ok {
			b.Fatal("asset not found")
		}
		i++
	}
}

func BenchmarkCatalogFindEncodingVariant(b *testing.B) {
	cat := benchmarkCatalog(b)
	paths := benchmarkAssetPaths()

	b.ReportAllocs()
	b.ResetTimer()
	i := 0
	for b.Loop() {
		if _, ok := cat.FindEncodingVariant(paths[i%len(paths)], "br"); !ok {
			b.Fatal("variant not found")
		}
		i++
	}
}

func BenchmarkCatalogListVariants(b *testing.B) {
	cat := benchmarkCatalog(b)
	paths := benchmarkAssetPaths()

	b.ReportAllocs()
	b.ResetTimer()
	i := 0
	for b.Loop() {
		if variants := cat.ListVariants(paths[i%len(paths)]); variants.Len() != benchmarkCatalogVariantsPerPath {
			b.Fatalf("expected %d variants, got %d", benchmarkCatalogVariantsPerPath, variants.Len())
		}
		i++
	}
}

func BenchmarkCatalogSnapshot(b *testing.B) {
	cat := benchmarkCatalog(b)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if snapshot := cat.Snapshot(); snapshot.Assets.Len() != benchmarkCatalogAssetCount {
			b.Fatalf("expected %d assets, got %d", benchmarkCatalogAssetCount, snapshot.Assets.Len())
		}
	}
}

func benchmarkCatalog(b *testing.B) *catalog.IndexedCatalog {
	b.Helper()
	cat := catalog.NewInMemoryCatalog()
	if err := cat.ReplaceCatalog(catalog.ReplaceCatalogInput{
		Assets:   benchmarkAssets(benchmarkCatalogAssetCount),
		Variants: benchmarkVariants(benchmarkCatalogAssetCount, benchmarkCatalogVariantsPerPath),
	}); err != nil {
		b.Fatal(err)
	}
	return cat
}

func benchmarkAssets(count int) *cxlist.List[*catalog.Asset] {
	assets := cxlist.NewListWithCapacity[*catalog.Asset](count)
	for i := range count {
		assets.Add(&catalog.Asset{
			Path:       fmt.Sprintf("asset-%05d.js", i),
			FullPath:   fmt.Sprintf("/assets/asset-%05d.js", i),
			Size:       4096,
			MediaType:  "application/javascript",
			SourceHash: fmt.Sprintf("hash-%05d", i),
			ETag:       fmt.Sprintf("\"hash-%05d\"", i),
		})
	}
	return assets
}

func benchmarkVariants(assetCount, variantsPerPath int) *cxlist.List[*catalog.Variant] {
	encodings := []string{"br", "gzip", "zstd", "identity"}
	variants := cxlist.NewListWithCapacity[*catalog.Variant](assetCount * variantsPerPath)
	for i := range assetCount {
		for j := range variantsPerPath {
			encoding := encodings[j%len(encodings)]
			variants.Add(&catalog.Variant{
				ID:           fmt.Sprintf("asset-%05d.js|encoding=%s", i, encoding),
				AssetPath:    fmt.Sprintf("asset-%05d.js", i),
				ArtifactPath: fmt.Sprintf("/artifacts/asset-%05d.js.%s", i, encoding),
				MediaType:    "application/javascript",
				SourceHash:   fmt.Sprintf("hash-%05d", i),
				ETag:         fmt.Sprintf("\"hash-%05d-%s\"", i, encoding),
				Encoding:     encoding,
			})
		}
	}
	return variants
}

func benchmarkAssetPaths() []string {
	paths := make([]string, 0, benchmarkCatalogAssetCount)
	for i := range benchmarkCatalogAssetCount {
		paths = append(paths, fmt.Sprintf("asset-%05d.js", i))
	}
	return paths
}
