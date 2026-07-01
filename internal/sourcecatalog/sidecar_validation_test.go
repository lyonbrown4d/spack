package sourcecatalog_test

import (
	"context"
	"encoding/base64"
	"log/slog"
	"path/filepath"
	"testing"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
)

func TestScannerSkipsInvalidExternalGzipSidecars(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, filepath.Join(root, "hero.png"), validPNGForSidecarTest(t))
	writeSourceFile(t, filepath.Join(root, "hero.png.gz"), []byte{0x78, 0xda, 0x01, 0x02})

	scanner := newScannerForTest(t, root, cxlist.NewList("gzip"))
	snapshot, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := snapshot.Assets.Get("hero.png"); !ok {
		t.Fatal("expected hero.png asset to be scanned")
	}
	if _, ok := snapshot.Assets.Get("hero.png.gz"); ok {
		t.Fatal("expected external hero.png.gz sidecar to be hidden from plain assets")
	}
	if _, ok := snapshot.Variants.Get("hero.png.gz"); ok {
		t.Fatal("expected default scanner to skip invalid external hero.png.gz sidecar")
	}
}

func TestScannerSkipsGzipSidecarsWithMismatchedDecodedMagic(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, filepath.Join(root, "hero.png"), validPNGForSidecarTest(t))
	writeCompressedSourceFile(t, filepath.Join(root, "hero.png.gz"), "gzip", []byte("not a png"))

	scanner := newScannerForTest(t, root, cxlist.NewList("gzip"))
	snapshot, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := snapshot.Variants.Get("hero.png.gz"); ok {
		t.Fatal("expected scanner to skip gzip sidecar whose decoded payload does not match .png magic")
	}
}

func TestScannerTrustsValidExternalGzipSidecars(t *testing.T) {
	root := t.TempDir()
	sourceBody := validPNGForSidecarTest(t)
	writeSourceFile(t, filepath.Join(root, "hero.png"), sourceBody)
	writeCompressedSourceFile(t, filepath.Join(root, "hero.png.gz"), "gzip", sourceBody)

	scanner := newScannerForTest(t, root, cxlist.NewList("gzip"))
	snapshot, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	variant, ok := snapshot.Variants.Get("hero.png.gz")
	if !ok || variant == nil {
		t.Fatal("expected valid external hero.png.gz sidecar to be registered")
	}
	if variant.Encoding != "gzip" {
		t.Fatalf("expected gzip encoding, got %q", variant.Encoding)
	}
}

func TestCompilerScannerSkipsInvalidExternalGzipSidecars(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, filepath.Join(root, "hero.png"), validPNGForSidecarTest(t))
	writeSourceFile(t, filepath.Join(root, "hero.png.gz"), []byte{0x78, 0xda, 0x01, 0x02})

	scanner := newCompilerScannerForTest(t, root, cxlist.NewList("gzip"))
	snapshot, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := snapshot.Assets.Get("hero.png"); !ok {
		t.Fatal("expected hero.png asset to be scanned")
	}
	if _, ok := snapshot.Assets.Get("hero.png.gz"); ok {
		t.Fatal("expected compiler scanner to drop external hero.png.gz as a plain asset")
	}
	if _, ok := snapshot.Variants.Get("hero.png.gz"); ok {
		t.Fatal("expected compiler scanner to skip invalid external hero.png.gz sidecar")
	}
}

func validPNGForSidecarTest(t *testing.T) []byte {
	t.Helper()
	body, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAAXNSR0IArs4c6QAAAARnQU1BAACxjwv8YQUAAAAJcEhZcwAADsMAAA7DAcdvqGQAAAANSURBVBhXY/jPwPAfAAUAAf+mXJtdAAAAAElFTkSuQmCC")
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func newCompilerScannerForTest(t *testing.T, root string, encodings *cxlist.List[string]) sourcecatalog.Scanner {
	t.Helper()

	cfg := config.DefaultConfigForTest()
	cfg.Assets.Root = root
	src, err := source.NewLocalFS(&cfg.Assets, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	registry := contentcoding.NewRegistry(contentcoding.Options{
		BrotliQuality: cfg.Compression.BrotliQuality,
		GzipLevel:     cfg.Compression.GzipLevel,
		ZstdLevel:     cfg.Compression.ZstdLevel,
	}, encodings)
	return sourcecatalog.NewCompilerScannerWithAssets(src, registry, &cfg.Assets)
}
