package server

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
)

func TestAssetDeliveryRuntimeSharesPreparedFileSources(t *testing.T) {
	root := t.TempDir()
	writeServerFileSourceFixture(t, root)
	cfg := config.DefaultConfigForTest()
	cfg.Assets.Root = root
	cfg.Compression.CacheDir = filepath.Join(t.TempDir(), "missing-cache")
	prepared := newPreparedService(
		&cfg,
		catalog.NewInMemoryCatalog(),
		slog.New(slog.DiscardHandler),
		nil,
		nil,
		nil,
	)
	t.Cleanup(func() {
		if err := prepared.stop(context.Background()); err != nil {
			t.Fatal(err)
		}
	})

	routeSource := newServerLocalDirectory(t, root)
	t.Cleanup(func() {
		if err := routeSource.Cleanup(); err != nil {
			t.Fatal(err)
		}
	})
	runtime := newAssetDeliveryRuntime(
		&cfg,
		assetRouteRuntime{
			logger:      slog.New(slog.DiscardHandler),
			prepared:    prepared,
			fileSources: newServerFileSourcesFromSource(routeSource),
		},
		nil,
		nil,
		nil,
		catalog.NewInMemoryCatalog(),
	)
	borrowed := runtime.fileSources
	if runtime.fileSources != prepared.fileSources {
		t.Fatal("asset delivery runtime did not share prepared file sources")
	}
	if runtime.fallbackFileSources != nil {
		t.Fatal("prepared asset delivery runtime must not own fallback sources")
	}
	if borrowed != prepared.fileSources {
		t.Fatal("asset delivery runtime selected route sources instead of prepared sources")
	}
}

func TestAssetDeliveryRuntimeFallbackCleansUpOnAppShutdown(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "assets")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	writeServerFileSourceFixture(t, root)
	cfg := config.DefaultConfigForTest()
	cfg.Assets.Root = root
	cfg.Compression.CacheDir = filepath.Join(parent, "missing-cache")

	runtime := newAssetDeliveryRuntime(
		&cfg,
		assetRouteRuntime{logger: slog.New(slog.DiscardHandler)},
		nil,
		nil,
		nil,
		catalog.NewInMemoryCatalog(),
	)
	if runtime.fileSources == nil || runtime.fallbackFileSources != runtime.fileSources {
		t.Fatal("expected an owned fallback source for nil prepared/testing runtime")
	}
	if runtime.resourceHints == nil || runtime.resourceHints.files != runtime.fileSources.first() {
		t.Fatal("resource hints did not borrow the fallback source")
	}

	app := fiber.New()
	registerAssetRoute(app, runtime)
	if err := app.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("remove fallback root after app shutdown: %v", err)
	}
}

func TestAssetDeliveryRuntimeBorrowedFallbackRemainsOpenAfterShutdown(t *testing.T) {
	root := t.TempDir()
	writeServerFileSourceFixture(t, root)
	borrowed := newServerLocalDirectory(t, root)
	t.Cleanup(func() {
		if err := borrowed.Cleanup(); err != nil {
			t.Fatal(err)
		}
	})
	cfg := config.DefaultConfigForTest()
	cfg.Assets.Root = root

	runtime := newAssetDeliveryRuntime(
		&cfg,
		assetRouteRuntime{
			logger:      slog.New(slog.DiscardHandler),
			fileSources: newServerFileSourcesFromSource(borrowed),
		},
		nil,
		nil,
		nil,
		catalog.NewInMemoryCatalog(),
	)
	if runtime.fallbackFileSources != nil {
		t.Fatal("borrowed route source must not become an owned fallback")
	}

	app := fiber.New()
	registerAssetRoute(app, runtime)
	if err := app.Shutdown(); err != nil {
		t.Fatal(err)
	}
	file, _, err := borrowed.OpenFile(filepath.Join(root, "app.js"))
	if err != nil {
		t.Fatalf("app shutdown closed borrowed route source: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
