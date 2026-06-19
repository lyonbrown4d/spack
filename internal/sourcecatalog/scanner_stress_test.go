//go:build stress

package sourcecatalog_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
)

func TestCatalogStressScan(t *testing.T) {
	count := stressAssetCount(t)
	root := t.TempDir()
	writeStressAssets(t, root, count)

	cfg := config.DefaultConfigForTest()
	cfg.Assets.Root = root
	src, err := source.NewLocalFS(&cfg.Assets, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	scanner := sourcecatalog.NewScanner(src, contentcoding.NewRegistry(contentcoding.Options{
		BrotliQuality: cfg.Compression.BrotliQuality,
		GzipLevel:     cfg.Compression.GzipLevel,
		ZstdLevel:     cfg.Compression.ZstdLevel,
	}, cfg.Compression.NormalizedEncodings()))

	var before runtime.MemStats
	var after runtime.MemStats
	runtime.ReadMemStats(&before)
	startedAt := time.Now()
	snapshot, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(startedAt)
	runtime.ReadMemStats(&after)

	if snapshot.Assets.Len() != count {
		t.Fatalf("expected %d scanned assets, got %d", count, snapshot.Assets.Len())
	}
	t.Logf(
		"catalog_stress assets=%d duration=%s total_bytes=%d heap_alloc_delta=%d heap_sys_delta=%d",
		count,
		elapsed,
		snapshot.TotalBytes,
		int64(after.HeapAlloc)-int64(before.HeapAlloc),
		int64(after.HeapSys)-int64(before.HeapSys),
	)
}

func stressAssetCount(t *testing.T) int {
	t.Helper()

	raw := os.Getenv("SPACK_STRESS_ASSET_COUNT")
	if raw == "" {
		return 100_000
	}
	count, err := strconv.Atoi(raw)
	if err != nil || count <= 0 {
		t.Fatalf("SPACK_STRESS_ASSET_COUNT must be a positive integer, got %q", raw)
	}
	return count
}

func writeStressAssets(t *testing.T, root string, count int) {
	t.Helper()

	payload := []byte("console.log('stress');\n")
	for i := range count {
		dir := filepath.Join(root, fmt.Sprintf("%04d", i/1000))
		if i%1000 == 0 {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				t.Fatal(err)
			}
		}
		path := filepath.Join(dir, fmt.Sprintf("asset-%07d.js", i))
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
