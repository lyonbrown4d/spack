package resolver_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/resolver"
)

func BenchmarkResolverResolveAsset(b *testing.B) {
	const payload = "console.log('app');"

	assetResolver := newResolverBenchmarkFixture(b, func(root string, cat catalog.Catalog) {
		upsertBenchmarkAsset(b, cat, "app.js", filepath.Join(root, "app.js"), "application/javascript", []byte(payload))
	})

	request := resolver.Request{Path: "app.js"}
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()

	for b.Loop() {
		result, err := assetResolver.Resolve(b.Context(), request)
		if err != nil {
			b.Fatal(err)
		}
		if result == nil || result.Asset == nil || result.Variant != nil {
			b.Fatalf("unexpected resolver result: %#v", result)
		}
	}
}

func BenchmarkResolverResolveEncodingVariant(b *testing.B) {
	const payload = "console.log('compressed');"
	const variantPayload = "br"

	assetResolver := newResolverBenchmarkFixture(b, func(root string, cat catalog.Catalog) {
		assetPath := filepath.Join(root, "app.js")
		variantPath := filepath.Join(root, "app.js.br")
		upsertBenchmarkAsset(b, cat, "app.js", assetPath, "application/javascript", []byte(payload))
		writeBenchmarkFile(b, variantPath, []byte(variantPayload))
		upsertBenchmarkVariant(b, cat, &catalog.Variant{
			ID:           "app.js|encoding=br",
			AssetPath:    "app.js",
			ArtifactPath: variantPath,
			MediaType:    "application/javascript",
			SourceHash:   testSourceHash,
			ETag:         "\"hash-1-br\"",
			Encoding:     "br",
		})
	})

	request := resolver.Request{
		Path:           "app.js",
		AcceptEncoding: "br,gzip;q=0.8",
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(variantPayload)))
	b.ResetTimer()

	for b.Loop() {
		result, err := assetResolver.Resolve(b.Context(), request)
		if err != nil {
			b.Fatal(err)
		}
		if result == nil || result.Variant == nil || result.Variant.Encoding != "br" {
			b.Fatalf("unexpected resolver result: %#v", result)
		}
	}
}

func BenchmarkResolverResolveImageVariant(b *testing.B) {
	const payload = "png"
	const variantPayload = "jpeg"

	assetResolver := newResolverBenchmarkFixture(b, func(root string, cat catalog.Catalog) {
		assetPath := filepath.Join(root, "hero.png")
		variantPath := filepath.Join(root, "hero.w640.fjpeg.jpg")
		upsertBenchmarkAsset(b, cat, "hero.png", assetPath, "image/png", []byte(payload))
		writeBenchmarkFile(b, variantPath, []byte(variantPayload))
		upsertBenchmarkVariant(b, cat, &catalog.Variant{
			ID:           "hero.png|width=640|format=jpeg",
			AssetPath:    "hero.png",
			ArtifactPath: variantPath,
			MediaType:    "image/jpeg",
			SourceHash:   testSourceHash,
			ETag:         "\"hash-1-w640-jpeg\"",
			Width:        640,
			Format:       "jpeg",
		})
	})

	request := resolver.Request{
		Path:   "hero.png",
		Width:  640,
		Accept: "image/jpeg,image/png;q=0.5",
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(variantPayload)))
	b.ResetTimer()

	for b.Loop() {
		result, err := assetResolver.Resolve(b.Context(), request)
		if err != nil {
			b.Fatal(err)
		}
		if result == nil || result.Variant == nil || result.Variant.Width != 640 || result.Variant.Format != "jpeg" {
			b.Fatalf("unexpected resolver result: %#v", result)
		}
	}
}

func newResolverBenchmarkFixture(
	b *testing.B,
	setup func(root string, cat catalog.Catalog),
) *resolver.Resolver {
	b.Helper()

	root := b.TempDir()
	cat := catalog.NewInMemoryCatalog()
	setup(root, cat)
	return resolver.NewResolverForTest(baseAssetsConfig(), cat, slog.New(slog.DiscardHandler))
}

func upsertBenchmarkAsset(
	b *testing.B,
	cat catalog.Catalog,
	assetPath string,
	sourcePath string,
	mediaType string,
	body []byte,
) {
	b.Helper()

	writeBenchmarkFile(b, sourcePath, body)
	if err := cat.UpsertAsset(&catalog.Asset{
		Path:       assetPath,
		FullPath:   sourcePath,
		MediaType:  mediaType,
		Size:       int64(len(body)),
		SourceHash: testSourceHash,
		ETag:       "\"hash-1\"",
	}); err != nil {
		b.Fatal(err)
	}
}

func writeBenchmarkFile(b *testing.B, path string, body []byte) {
	b.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		b.Fatal(err)
	}
}

func upsertBenchmarkVariant(b *testing.B, cat catalog.Catalog, variant *catalog.Variant) {
	b.Helper()

	if err := cat.UpsertVariant(variant); err != nil {
		b.Fatal(err)
	}
}
