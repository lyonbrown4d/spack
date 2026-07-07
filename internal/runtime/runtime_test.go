package runtime_test

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/runtime"
	"github.com/lyonbrown4d/spack/internal/server"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestBuildCatalogAssetSetsHashETagAndMtime(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "app.js")
	payload := []byte("console.log('runtime');")
	if err := os.WriteFile(assetPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	modTime := time.Unix(1_720_000_321, 0).UTC()
	if err := os.Chtimes(assetPath, modTime, modTime); err != nil {
		t.Fatal(err)
	}

	fileInfo, err := os.Stat(assetPath)
	if err != nil {
		t.Fatal(err)
	}

	asset, err := runtime.BuildCatalogAssetForTest(source.File{
		Path:     "app.js",
		FullPath: assetPath,
		Size:     fileInfo.Size(),
		ModTime:  fileInfo.ModTime(),
	})
	if err != nil {
		t.Fatal(err)
	}

	if asset.Path != "app.js" {
		t.Fatalf("expected asset path app.js, got %q", asset.Path)
	}
	if asset.SourceHash == "" {
		t.Fatal("expected source hash to be populated")
	}
	if asset.ETag != `"`+asset.SourceHash+`"` {
		t.Fatalf("expected etag derived from source hash, got %q", asset.ETag)
	}
	if got := asset.Metadata.GetOrDefault("mtime_unix", ""); got != "1720000321" {
		t.Fatalf("expected mtime metadata to be preserved, got %q", got)
	}
	if got := asset.Metadata.GetOrDefault("last_modified_http", ""); got != modTime.Format(http.TimeFormat) {
		t.Fatalf("expected http last-modified metadata to be preserved, got %q", got)
	}
}

func TestCatalogReadyAttrsIncludeCacheAndCompressionState(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	cat := catalog.NewInMemoryCatalog()
	bodyCache := assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, slog.New(slog.DiscardHandler))

	if err := cat.UpsertAsset(&catalog.Asset{Path: "app.js", MediaType: "text/javascript"}); err != nil {
		t.Fatal(err)
	}
	if err := cat.UpsertVariant(&catalog.Variant{ID: "app.js|encoding=br", AssetPath: "app.js", Encoding: "br"}); err != nil {
		t.Fatal(err)
	}

	attrs := runtime.CatalogReadyAttrsForTest(cat, bodyCache, assetcache.WarmStats{Entries: 2, Bytes: 128}, 2048, 50*time.Millisecond)
	attrMap := cxmapping.NewMapWithCapacity[string, any](attrs.Len())
	attrs.Range(func(_ int, attr slog.Attr) bool {
		attrMap.Set(attr.Key, attr.Value.Any())
		return true
	})

	if got, _ := attrMap.Get("assets"); got != int64(1) {
		t.Fatalf("expected assets attr to be 1, got %#v", got)
	}
	if got, _ := attrMap.Get("variants"); got != int64(1) {
		t.Fatalf("expected variants attr to be 1, got %#v", got)
	}
	if got, _ := attrMap.Get("memory_cache_enable"); got != true {
		t.Fatalf("expected memory_cache_enable true, got %#v", got)
	}
	assertCountAttr(t, attrMap, "asset_media_types", "text/javascript", 1)
	assertCountAttr(t, attrMap, "variant_encodings", "br", 1)
}

func TestHTTPListenConfigDisablesPrefork(t *testing.T) {
	listenCfg := runtime.MainHTTPListenConfigForTest()
	if listenCfg.EnablePrefork {
		t.Fatal("expected prefork to be disabled")
	}
	if !listenCfg.DisableStartupMessage {
		t.Fatal("expected startup message to stay disabled")
	}
}

func TestBootstrapCatalogRecordsAOTStartupMetrics(t *testing.T) {
	assetRoot := t.TempDir()
	indexBody := []byte("<h1>ok</h1>")
	appBody := []byte("console.log('ok');")
	indexPath := filepath.Join(assetRoot, "index.html")
	appPath := filepath.Join(assetRoot, "assets", "app.js")
	writeRuntimeTestFile(t, indexPath, indexBody)
	writeRuntimeTestFile(t, appPath, appBody)

	bundlePath := filepath.Join(t.TempDir(), "app.spack")
	if _, err := spackbundle.Write(context.Background(), spackbundle.WriteOptions{
		Output: bundlePath,
		Root:   assetRoot,
		Files: []spackbundle.File{
			{Path: "index.html", FullPath: indexPath, Kind: "asset", MediaType: "text/html"},
			{Path: "assets/app.js", FullPath: appPath, Kind: "asset", MediaType: "text/javascript"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfigForTest()
	cfg.Assets.Root = bundlePath
	cfg.HTTP.MemoryCache.Warmup = false
	logger := slog.New(slog.DiscardHandler)
	src, err := source.NewLocalFS(&cfg.Assets, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := src.Cleanup(); err != nil {
			t.Fatal(err)
		}
	})

	cat := catalog.NewInMemoryCatalog()
	catMetrics := catalog.NewRuntimeMetrics()
	serverMetrics := server.NewRuntimeMetrics()
	bodyCache := assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, logger)
	prepared := server.NewPreparedServiceWithRuntimeMetricsForTest(&cfg, logger, cat, serverMetrics)
	scanner := sourcecatalog.NewScannerWithAssets(
		src,
		contentcoding.NewRegistry(contentcoding.Options{}, cfg.Compression.NormalizedEncodings()),
		&cfg.Assets,
	)

	if err := runtime.BootstrapCatalogForTest(context.Background(), &cfg, scanner, cat, catMetrics, serverMetrics, bodyCache, prepared, logger); err != nil {
		t.Fatal(err)
	}

	assertRuntimeStartupMetrics(t, catMetrics, serverMetrics, len(indexBody)+len(appBody))
}

func assertRuntimeStartupMetrics(
	t *testing.T,
	catMetrics *catalog.RuntimeMetrics,
	serverMetrics *server.RuntimeMetrics,
	wantBytes int,
) {
	t.Helper()
	assertGaugeEqual(t, catMetrics.SourceMode.WithLabelValues("aot"), 1, "aot source mode")
	assertGaugeEqual(t, catMetrics.SourceBundleExtractionFiles, 2, "extracted bundle files")
	assertGaugeEqual(t, catMetrics.SourceBundleExtractionBytes, float64(wantBytes), "extracted bundle bytes")
	assertGaugeEqual(t, catMetrics.AssetsCurrent, 2, "catalog assets")
	assertGaugeAtLeastZero(t, catMetrics.CatalogScanDuration, "catalog scan duration")
	assertGaugeAtLeastZero(t, serverMetrics.StartupDuration, "startup duration")
	assertGaugeEqual(t, serverMetrics.StartupPhase.WithLabelValues("ready"), 1, "ready startup phase")
	assertGaugeEqual(t, serverMetrics.Readiness.WithLabelValues("ready"), 1, "ready readiness")
	assertGaugeEqual(t, serverMetrics.PreparedSnapshotRoutesCurrent, 2, "prepared routes")
	assertGaugeEqual(t, serverMetrics.PreparedSnapshotBodyEntriesCurrent, 2, "prepared body entries")
	assertGaugeEqual(t, serverMetrics.PreparedSnapshotBodyBytesCurrent, float64(wantBytes), "prepared body bytes")
	assertGaugeAtLeastZero(t, serverMetrics.PreparedSnapshotDuration, "prepared snapshot duration")
}

func assertGaugeEqual(t *testing.T, collector prometheus.Collector, want float64, label string) {
	t.Helper()
	if got := testutil.ToFloat64(collector); got != want {
		t.Fatalf("expected %s gauge %v, got %v", label, want, got)
	}
}

func assertGaugeAtLeastZero(t *testing.T, collector prometheus.Collector, label string) {
	t.Helper()
	if got := testutil.ToFloat64(collector); got < 0 {
		t.Fatalf("expected %s to be non-negative, got %v", label, got)
	}
}
func assertCountAttr(t *testing.T, attrs *cxmapping.Map[string, any], attrKey, countKey string, want int) {
	t.Helper()

	raw, ok := attrs.Get(attrKey)
	if !ok {
		t.Fatalf("expected %s attr", attrKey)
	}
	counts, ok := raw.(map[string]int)
	if !ok {
		t.Fatalf("expected %s to be map[string]int, got %#v", attrKey, raw)
	}
	if got := counts[countKey]; got != want {
		t.Fatalf("expected %s[%s] to be %d, got %d", attrKey, countKey, want, got)
	}
}

func writeRuntimeTestFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}
