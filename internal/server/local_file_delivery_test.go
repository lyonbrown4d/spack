package server_test

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
)

func TestServerSkipsMissingCompressionCacheDirWarning(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	cfg := config.DefaultConfigForTest()
	cfg.Assets.Root = t.TempDir()
	cfg.Compression.CacheDir = filepath.Join(t.TempDir(), "missing-cache")

	cat := catalog.NewInMemoryCatalog()
	app := newHTTPTestApp(
		t,
		&cfg,
		logger,
		cat,
		assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, slog.New(slog.DiscardHandler)),
		resolver.NewResolverForTest(&cfg.Assets, cat, slog.New(slog.DiscardHandler)),
	)
	t.Cleanup(func() {
		if err := app.Shutdown(); err != nil {
			t.Fatalf("shutdown test app: %v", err)
		}
	})

	if strings.Contains(logs.String(), "Local file source unavailable") {
		t.Fatalf("expected missing optional compression cache dir not to warn, got logs %q", logs.String())
	}
}
