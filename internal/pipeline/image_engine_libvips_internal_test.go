//go:build spack_libvips

package pipeline

import (
	"log/slog"
	"testing"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/config"
)

func TestLibvipsImageEngineGeneratesModernFormatsBatch(t *testing.T) {
	engine := newLibvipsImageEngine(
		&config.Image{JPEGQuality: 70, MaxMemoryBytes: 64 * 1024 * 1024},
		slog.New(slog.DiscardHandler),
		imageEngineTelemetry{},
	)
	if !engine.SupportsSourceMediaType("image/png") {
		t.Fatal("expected libvips engine to support png sources")
	}

	sourcePath := writeInternalPNGFixture(t, t.TempDir(), 64, 64)
	results, err := engine.GenerateBatch(imageGenerateBatchRequest{
		SourcePath:      sourcePath,
		SourceMediaType: "image/png",
		Variants: cxlist.NewList(
			imageVariantGenerateRequest{TargetFormat: "webp", TargetWidth: 32},
			imageVariantGenerateRequest{TargetFormat: "avif", TargetWidth: 32},
		),
		Encode: imageEncodeOptions{JPEGQuality: 70},
		Limits: imageGenerateLimits{MaxMemoryBytes: 64 * 1024 * 1024},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertLibvipsBatchFormat(t, results, "webp", "image/webp")
	assertLibvipsBatchFormat(t, results, "avif", "image/avif")
}

func TestLibvipsImageEngineRejectsMemoryBudget(t *testing.T) {
	engine := newLibvipsImageEngine(
		&config.Image{MaxMemoryBytes: 16},
		slog.New(slog.DiscardHandler),
		imageEngineTelemetry{},
	)

	sourcePath := writeInternalPNGFixture(t, t.TempDir(), 16, 16)
	_, err := engine.GenerateBatch(imageGenerateBatchRequest{
		SourcePath:      sourcePath,
		SourceMediaType: "image/png",
		Variants: cxlist.NewList(
			imageVariantGenerateRequest{TargetFormat: "webp", TargetWidth: 8},
		),
		Limits: imageGenerateLimits{MaxMemoryBytes: 16},
	})
	if !IsVariantSkipped(err) {
		t.Fatalf("expected memory-limited libvips source to be skipped, got %v", err)
	}
}

func assertLibvipsBatchFormat(
	t *testing.T,
	results *cxlist.List[imageGenerateResult],
	format string,
	mediaType string,
) {
	t.Helper()

	var result imageGenerateResult
	var ok bool
	results.Range(func(_ int, value imageGenerateResult) bool {
		if value.TargetFormat != format {
			return true
		}
		result = value
		ok = true
		return false
	})
	if !ok {
		t.Fatalf("expected %s result, got %#v", format, results.Values())
	}
	if result.MediaType != mediaType {
		t.Fatalf("expected %s media type %q, got %q", format, mediaType, result.MediaType)
	}
	if result.Width != 32 || result.Height <= 0 || len(result.Payload) == 0 {
		t.Fatalf("unexpected %s result: %#v", format, result)
	}
}
