//go:build spack_libvips

package pipeline

import (
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"os"
	"testing"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/config"
)

func TestLibvipsImageEngineRejectsSourceByteLimit(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeInternalTempBytes(t, root, make([]byte, 128))

	engine := newLibvipsImageEngine(&config.Image{}, slog.New(slog.DiscardHandler), imageEngineTelemetry{})
	_, err := engine.GenerateBatch(imageGenerateBatchRequest{
		Context:     t.Context(),
		SourcePath:  sourcePath,
		SourceBytes: internalFileSize(t, sourcePath),
		Variants: cxlist.NewList(imageVariantGenerateRequest{
			TargetFormat: "png",
			TargetWidth:  16,
		}),
		Limits: imageGenerateLimits{MaxSourceBytes: 64},
	})
	if !IsVariantSkipped(err) {
		t.Fatalf("expected byte-limited source to be skipped, got %v", err)
	}
}

func TestLibvipsImageEngineRejectsSourcePixelLimit(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeInternalPNGFixture(t, root, 4, 4)

	engine := newLibvipsImageEngine(&config.Image{}, slog.New(slog.DiscardHandler), imageEngineTelemetry{})
	_, err := engine.GenerateBatch(imageGenerateBatchRequest{
		Context:     t.Context(),
		SourcePath:  sourcePath,
		SourceBytes: internalFileSize(t, sourcePath),
		Variants: cxlist.NewList(imageVariantGenerateRequest{
			TargetFormat: "png",
			TargetWidth:  2,
		}),
		Limits: imageGenerateLimits{MaxSourcePixels: 8},
	})
	if !IsVariantSkipped(err) {
		t.Fatalf("expected pixel-limited source to be skipped, got %v", err)
	}
}

func writeInternalPNGFixture(t *testing.T, root string, width, height int) string {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: 255})
		}
	}
	file, err := os.CreateTemp(root, "image-*.png")
	if err != nil {
		t.Fatal(err)
	}
	defer closeInternalTestFile(t, file)
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
	return file.Name()
}

func writeInternalTempBytes(t *testing.T, root string, body []byte) string {
	t.Helper()

	file, err := os.CreateTemp(root, "source-*")
	if err != nil {
		t.Fatal(err)
	}
	defer closeInternalTestFile(t, file)
	if _, err := file.Write(body); err != nil {
		t.Fatal(err)
	}
	return file.Name()
}

func internalFileSize(t *testing.T, path string) int64 {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

func closeInternalTestFile(t *testing.T, file *os.File) {
	t.Helper()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
