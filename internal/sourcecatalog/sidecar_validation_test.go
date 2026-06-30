package sourcecatalog_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
)

func TestScannerSkipsExternalGzipSidecarsByDefault(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, filepath.Join(root, "hero.webp"), []byte("RIFF....WEBPVP8 "))
	writeSourceFile(t, filepath.Join(root, "hero.webp.gz"), []byte{0x78, 0xda, 0x01, 0x02})

	scanner := newScannerForTest(t, root, cxlist.NewList("gzip"))
	snapshot, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := snapshot.Assets.Get("hero.webp"); !ok {
		t.Fatal("expected hero.webp asset to be scanned")
	}
	if _, ok := snapshot.Assets.Get("hero.webp.gz"); ok {
		t.Fatal("expected external hero.webp.gz sidecar to be hidden from plain assets")
	}
	if _, ok := snapshot.Variants.Get("hero.webp.gz"); ok {
		t.Fatal("expected default scanner to skip external hero.webp.gz sidecar")
	}
}

func TestCompilerScannerSkipsExternalGzipSidecars(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, filepath.Join(root, "hero.webp"), []byte("RIFF....WEBPVP8 "))
	writeSourceFile(t, filepath.Join(root, "hero.webp.gz"), []byte{0x78, 0xda, 0x01, 0x02})

	scanner := newCompilerScannerForTest(t, root, cxlist.NewList("gzip"))
	snapshot, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := snapshot.Assets.Get("hero.webp"); !ok {
		t.Fatal("expected hero.webp asset to be scanned")
	}
	if _, ok := snapshot.Assets.Get("hero.webp.gz"); ok {
		t.Fatal("expected compiler scanner to drop external hero.webp.gz as a plain asset")
	}
	if _, ok := snapshot.Variants.Get("hero.webp.gz"); ok {
		t.Fatal("expected compiler scanner to skip external hero.webp.gz sidecar")
	}
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
