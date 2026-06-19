package runtime_test

import (
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/runtime"
	"github.com/lyonbrown4d/spack/internal/source"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
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

	attrs := runtime.CatalogReadyAttrsForTest(&cfg, cat, bodyCache, assetcache.WarmStats{Entries: 2, Bytes: 128}, 2048, 50*time.Millisecond)
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
	if got, _ := attrMap.Get("compression_mode"); got != cfg.Compression.NormalizedMode() {
		t.Fatalf("expected compression_mode %q, got %#v", cfg.Compression.NormalizedMode(), got)
	}
	assertCountAttr(t, attrMap, "asset_media_types", "text/javascript", 1)
	assertCountAttr(t, attrMap, "variant_encodings", "br", 1)
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

func TestHTTPListenConfigUsesPreforkSetting(t *testing.T) {
	cfg := config.DefaultConfigForTest()

	listenCfg := runtime.MainHTTPListenConfigForTest(&cfg)
	if listenCfg.EnablePrefork {
		t.Fatal("expected prefork to be disabled by default")
	}
	if !listenCfg.DisableStartupMessage {
		t.Fatal("expected startup message to stay disabled")
	}

	cfg.HTTP.Prefork = true
	listenCfg = runtime.MainHTTPListenConfigForTest(&cfg)
	if !listenCfg.EnablePrefork {
		t.Fatal("expected prefork to be enabled when configured")
	}
}
