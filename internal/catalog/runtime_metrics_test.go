package catalog_test

import (
	"testing"

	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRuntimeMetricsSyncCatalogAndSourceBytes(t *testing.T) {
	metrics := catalog.NewRuntimeMetrics()
	cat := catalog.NewInMemoryCatalog()

	if err := cat.UpsertAsset(&catalog.Asset{Path: "app.js"}); err != nil {
		t.Fatal(err)
	}
	if err := cat.UpsertVariant(&catalog.Variant{
		ID:        "app.js|encoding=br",
		AssetPath: "app.js",
	}); err != nil {
		t.Fatal(err)
	}

	metrics.SyncCatalog(cat)
	metrics.SetSourceBytes(2048)

	if got := testutil.ToFloat64(metrics.AssetsCurrent); got != 1 {
		t.Fatalf("expected assets gauge 1, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.VariantsCurrent); got != 1 {
		t.Fatalf("expected variants gauge 1, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.SourceBytesCurrent); got != 2048 {
		t.Fatalf("expected source bytes gauge 2048, got %v", got)
	}
}
