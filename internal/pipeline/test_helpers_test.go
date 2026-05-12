package pipeline_test

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/lyonbrown4d/spack/internal/artifact"
)

func newTestStore(root string) artifact.Store {
	return &testStore{root: root}
}

type testStore struct {
	root string
}

func (s *testStore) Root() string {
	return s.root
}

func (s *testStore) PathFor(assetPath, sourceHash, namespace, suffix string) string {
	cleanPath := filepath.Clean(filepath.FromSlash(assetPath))
	return filepath.Join(s.root, namespace, sourceHash, cleanPath+suffix)
}

func (s *testStore) Write(path string, data []byte) error {
	// #nosec G703 -- test paths are created under t.TempDir().
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create test artifact directory: %w", err)
	}

	tmpPath := path + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	// #nosec G703 -- test paths are created under t.TempDir().
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write test artifact temp file: %w", err)
	}
	// #nosec G703 -- test paths are created under t.TempDir().
	if err := os.Rename(tmpPath, path); err != nil {
		// #nosec G703 -- test paths are created under t.TempDir().
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return errors.Join(fmt.Errorf("rename test artifact temp file: %w", err), fmt.Errorf("cleanup test artifact temp file: %w", removeErr))
		}
		return fmt.Errorf("rename test artifact temp file: %w", err)
	}
	return nil
}

func writeJPEGFixture(t *testing.T, path string, width, height int) {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{
				R: uint8(x % 255),
				G: uint8(y % 255),
				B: uint8((x + y) % 255),
				A: 255,
			})
		}
	}

	// #nosec G304 -- test paths are created under t.TempDir().
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestFile(t, file)

	if err := jpeg.Encode(file, img, &jpeg.Options{Quality: 92}); err != nil {
		t.Fatal(err)
	}
}

func writePNGFixture(t *testing.T, path string, width, height int) {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{
				R: uint8(x % 255),
				G: uint8(y % 255),
				B: uint8((x + y) % 255),
				A: 255,
			})
		}
	}

	// #nosec G304 -- test paths are created under t.TempDir().
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestFile(t, file)

	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(file, img); err != nil {
		t.Fatal(err)
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

func closeTestFile(t *testing.T, file *os.File) {
	t.Helper()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
