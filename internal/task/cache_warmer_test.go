package task_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/task"
)

func TestWarmCacheHotsetWarmsConfiguredAssetsAndVariants(t *testing.T) {
	root := t.TempDir()
	cache := newWarmCacheForTest(t)
	cat := catalog.NewInMemoryCatalog()

	fixture := newWarmCacheFixture(t, root, cat)

	report, err := task.WarmCacheHotsetForTest(context.Background(), newWarmCacheConfig(), cat, cache)
	if err != nil {
		t.Fatal(err)
	}

	if report.Assets != 3 {
		t.Fatalf("expected 3 warmed assets, got %d", report.Assets)
	}
	if report.Variants != 1 {
		t.Fatalf("expected 1 warmed variant, got %d", report.Variants)
	}
	if report.LoadedEntries != 4 {
		t.Fatalf("expected 4 loaded entries, got %d", report.LoadedEntries)
	}
	expectedBytes := fixture.expectedBytes()
	if report.LoadedBytes != expectedBytes {
		t.Fatalf("expected loaded bytes %d, got %d", expectedBytes, report.LoadedBytes)
	}
}

type warmCacheFixture struct {
	entryBody    []byte
	fallbackBody []byte
	robotsBody   []byte
	variantBody  []byte
}

func newWarmCacheForTest(t *testing.T) *assetcache.Cache {
	t.Helper()

	cache := assetcache.NewCacheWithSettingsForTest(config.MemoryCache{
		Enable:      true,
		Warmup:      true,
		MaxEntries:  16,
		MaxFileSize: 64 * 1024,
		TTL:         "5m",
	}, slog.New(slog.DiscardHandler), nil, &asyncx.Settings{Size: 2})
	if startErr := assetcache.StartForTest(cache); startErr != nil {
		t.Fatal(startErr)
	}
	return cache
}

func newWarmCacheFixture(t *testing.T, root string, cat catalog.Catalog) warmCacheFixture {
	t.Helper()

	fixture := warmCacheFixture{
		entryBody:    []byte("<html>index</html>"),
		fallbackBody: []byte("<html>app</html>"),
		robotsBody:   []byte("User-agent: *\nAllow: /\n"),
		variantBody:  []byte("compressed"),
	}

	entryPath := filepath.Join(root, "index.html")
	fallbackPath := filepath.Join(root, "app.html")
	robotsPath := filepath.Join(root, "robots.txt")
	variantPath := filepath.Join(root, "cache", "index.html.br")
	writeFileForTest(t, entryPath, fixture.entryBody)
	writeFileForTest(t, fallbackPath, fixture.fallbackBody)
	writeFileForTest(t, robotsPath, fixture.robotsBody)
	writeFileForTest(t, variantPath, fixture.variantBody)

	upsertAssetForTest(t, cat, &catalog.Asset{
		Path:     "index.html",
		FullPath: entryPath,
		Size:     int64(len(fixture.entryBody)),
	})
	upsertAssetForTest(t, cat, &catalog.Asset{
		Path:     "app.html",
		FullPath: fallbackPath,
		Size:     int64(len(fixture.fallbackBody)),
	})
	upsertAssetForTest(t, cat, &catalog.Asset{
		Path:     "robots.txt",
		FullPath: robotsPath,
		Size:     int64(len(fixture.robotsBody)),
	})
	if upsertErr := cat.UpsertVariant(&catalog.Variant{
		ID:           "index.html|encoding=br",
		AssetPath:    "index.html",
		ArtifactPath: variantPath,
		Size:         int64(len(fixture.variantBody)),
		Encoding:     "br",
	}); upsertErr != nil {
		t.Fatal(upsertErr)
	}

	return fixture
}

func newWarmCacheConfig() *config.Config {
	return &config.Config{
		HTTP: config.HTTP{
			MemoryCache: config.MemoryCache{
				Enable:      true,
				Warmup:      true,
				MaxEntries:  16,
				MaxFileSize: 64 * 1024,
				TTL:         "5m",
			},
		},
		Assets: config.Assets{
			Entry: "index.html",
			Fallback: config.Fallback{
				Target: "app.html",
			},
		},
		Robots: config.Robots{
			Enable: true,
		},
	}
}

func (f warmCacheFixture) expectedBytes() int64 {
	return int64(len(f.entryBody) + len(f.fallbackBody) + len(f.robotsBody) + len(f.variantBody))
}
