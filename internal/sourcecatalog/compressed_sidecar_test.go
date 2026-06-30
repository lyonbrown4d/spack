package sourcecatalog_test

import (
	"testing"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
)

func writeCompressedSourceFile(t *testing.T, path, encoding string, raw []byte) []byte {
	t.Helper()

	cfg := config.DefaultConfigForTest()
	strategy, ok := contentcoding.NewRegistry(contentcoding.Options{
		BrotliQuality: cfg.Compression.BrotliQuality,
		GzipLevel:     cfg.Compression.GzipLevel,
		ZstdLevel:     cfg.Compression.ZstdLevel,
	}, cxlist.NewList(encoding)).Lookup(encoding)
	if !ok {
		t.Fatalf("expected compression strategy %q", encoding)
	}
	body, err := strategy.Compress(raw)
	if err != nil {
		t.Fatal(err)
	}
	writeSourceFile(t, path, body)
	return body
}
