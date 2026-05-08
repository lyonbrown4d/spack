package resolver_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"

	"github.com/daiyuang/spack/internal/catalog"
	"github.com/daiyuang/spack/internal/config"
	"github.com/daiyuang/spack/internal/resolver"
)

func TestParseAcceptEncodingPriority(t *testing.T) {
	got := resolver.ParseAcceptEncodingForTest("gzip;q=0.8, zstd;q=0.9, br;q=1.0")
	if !slices.Equal(got.Values(), []string{"br", "zstd", "gzip"}) {
		t.Fatalf("unexpected encodings: %#v", got)
	}
}

func TestParseAcceptEncodingParameterOrderingAndWhitespace(t *testing.T) {
	got := resolver.ParseAcceptEncodingWithSupportedForTest(
		"gzip;level=9;q=0.4 ;x=1, br;Q=0.9;foo=bar, zstd;foo=bar;q=0.5",
		stringToList("gzip,br,zstd"),
	)
	if !slices.Equal(got.Values(), []string{"br", "zstd", "gzip"}) {
		t.Fatalf("unexpected encodings: %#v", got)
	}
}

func TestParseAcceptEncodingWildcard(t *testing.T) {
	got := resolver.ParseAcceptEncodingForTest("gzip;q=0, *;q=0.5")
	if !slices.Equal(got.Values(), []string{"br", "zstd"}) {
		t.Fatalf("unexpected encodings: %#v", got)
	}
}

func TestParseAcceptEncodingUsesConfiguredSupportOrder(t *testing.T) {
	got := resolver.ParseAcceptEncodingWithSupportedForTest("gzip;q=1, zstd;q=1, br;q=1", stringToList("zstd,gzip"))
	if !slices.Equal(got.Values(), []string{"zstd", "gzip"}) {
		t.Fatalf("unexpected encodings with configured support order: %#v", got)
	}
}

func TestResolverSelectsCompressedVariant(t *testing.T) {
	sourcePath, cat, assetResolver := newResolverFixture(t, "index.html", "text/html; charset=utf-8", []byte("<html>origin</html>"), spaAssetsConfig())
	variantPath := filepath.Join(filepath.Dir(sourcePath), "index.html.br")
	writeTestFile(t, variantPath, []byte("compressed"))
	upsertTestVariant(t, cat, &catalog.Variant{
		ID:           "index.html.br",
		AssetPath:    "index.html",
		ArtifactPath: variantPath,
		MediaType:    "text/html; charset=utf-8",
		SourceHash:   testSourceHash,
		ETag:         "\"hash-1-br\"",
		Encoding:     "br",
	})

	result, err := assetResolver.Resolve(context.Background(), resolver.Request{
		Path:           "index.html",
		AcceptEncoding: "br,gzip",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ContentEncoding != "br" {
		t.Fatalf("expected br encoding, got %q", result.ContentEncoding)
	}
	if result.ETag != "\"hash-1-br\"" {
		t.Fatalf("expected variant etag, got %q", result.ETag)
	}
}

func TestResolverSkipsDisabledEncoding(t *testing.T) {
	sourcePath, cat, _ := newResolverFixture(t, "index.html", "text/html; charset=utf-8", []byte("<html>origin</html>"), spaAssetsConfig())
	variantPath := filepath.Join(filepath.Dir(sourcePath), "index.html.zst")
	writeTestFile(t, variantPath, []byte("compressed-zstd"))
	upsertTestVariant(t, cat, &catalog.Variant{
		ID:           "index.html.zst",
		AssetPath:    "index.html",
		ArtifactPath: variantPath,
		MediaType:    "text/html; charset=utf-8",
		SourceHash:   testSourceHash,
		ETag:         "\"hash-1-zstd\"",
		Encoding:     "zstd",
	})

	compression := config.DefaultConfigForTest().Compression
	compression.Encodings = "br,gzip"
	assetResolver := resolver.NewResolverWithCompressionForTest(spaAssetsConfig(), &compression, cat, slog.New(slog.DiscardHandler))

	result, err := assetResolver.Resolve(context.Background(), resolver.Request{
		Path:           "index.html",
		AcceptEncoding: "zstd,br;q=0.5,gzip;q=0.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ContentEncoding != "" {
		t.Fatalf("expected disabled zstd variant to be skipped, got %q", result.ContentEncoding)
	}
	if !slices.Equal(result.PreferredEncodings.Values(), []string{"br", "gzip"}) {
		t.Fatalf("expected fallback preferred encodings [br gzip], got %#v", result.PreferredEncodings.Values())
	}
}

func TestResolverSelectsZstdVariant(t *testing.T) {
	sourcePath, cat, assetResolver := newResolverFixture(t, "index.html", "text/html; charset=utf-8", []byte("<html>origin</html>"), spaAssetsConfig())
	variantPath := filepath.Join(filepath.Dir(sourcePath), "index.html.zst")
	writeTestFile(t, variantPath, []byte("compressed-zstd"))
	upsertTestVariant(t, cat, &catalog.Variant{
		ID:           "index.html.zst",
		AssetPath:    "index.html",
		ArtifactPath: variantPath,
		MediaType:    "text/html; charset=utf-8",
		SourceHash:   testSourceHash,
		ETag:         "\"hash-1-zstd\"",
		Encoding:     "zstd",
	})

	result, err := assetResolver.Resolve(context.Background(), resolver.Request{
		Path:           "index.html",
		AcceptEncoding: "zstd,br;q=0.5,gzip;q=0.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ContentEncoding != "zstd" {
		t.Fatalf("expected zstd encoding, got %q", result.ContentEncoding)
	}
	if result.ETag != "\"hash-1-zstd\"" {
		t.Fatalf("expected zstd variant etag, got %q", result.ETag)
	}
}
