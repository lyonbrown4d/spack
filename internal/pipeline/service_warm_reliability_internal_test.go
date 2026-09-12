package pipeline

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
)

type warmReliabilityStage struct {
	variant *catalog.Variant
	err     error
}

func (warmReliabilityStage) Name() string {
	return "test"
}

func (warmReliabilityStage) Plan(asset *catalog.Asset, _ Request) *cxlist.List[Task] {
	return cxlist.NewList(Task{AssetPath: asset.Path})
}

func (stage warmReliabilityStage) Execute(context.Context, Task, *catalog.Asset) (*catalog.Variant, error) {
	return stage.variant, stage.err
}

type blockingMetricsCatalog struct {
	catalog.Catalog
	countStarted chan struct{}
	releaseCount chan struct{}
}

func (c *blockingMetricsCatalog) AssetCount() int {
	select {
	case c.countStarted <- struct{}{}:
	default:
	}
	<-c.releaseCount
	return c.Catalog.AssetCount()
}

func TestWarmReturnsStageFailure(t *testing.T) {
	wantErr := errors.New("generate failed")
	svc := newWarmReliabilityService(t, catalog.NewInMemoryCatalog(), warmReliabilityStage{err: wantErr}, nil)

	err := svc.Warm(t.Context())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected stage failure, got %v", err)
	}
}

func TestWarmTreatsVariantSkippedAsSuccess(t *testing.T) {
	svc := newWarmReliabilityService(t, catalog.NewInMemoryCatalog(), warmReliabilityStage{err: ErrVariantSkipped}, nil)

	if err := svc.Warm(t.Context()); err != nil {
		t.Fatalf("expected skipped variant to succeed, got %v", err)
	}
}

func TestWarmReturnsCatalogUpsertFailure(t *testing.T) {
	base := catalog.NewInMemoryCatalog()
	svc := newWarmReliabilityService(t, base, warmReliabilityStage{variant: &catalog.Variant{
		ID:        "missing.js|encoding=br",
		AssetPath: "missing.js",
		Encoding:  "br",
	}}, nil)

	err := svc.Warm(t.Context())
	if !errors.Is(err, catalog.ErrAssetNotFound) {
		t.Fatalf("expected catalog upsert failure, got %v", err)
	}
}

func TestWarmWaitsForCatalogMetricsSync(t *testing.T) {
	base := catalog.NewInMemoryCatalog()
	blocking := &blockingMetricsCatalog{
		Catalog:      base,
		countStarted: make(chan struct{}, 1),
		releaseCount: make(chan struct{}),
	}
	svc := newWarmReliabilityService(t, blocking, warmReliabilityStage{err: ErrVariantSkipped}, catalog.NewRuntimeMetrics())

	done := make(chan error, 1)
	go func() {
		done <- svc.Warm(t.Context())
	}()

	select {
	case <-blocking.countStarted:
	case <-time.After(time.Second):
		t.Fatal("catalog metrics sync did not start")
	}
	select {
	case err := <-done:
		close(blocking.releaseCount)
		t.Fatalf("Warm returned before catalog metrics sync completed: %v", err)
	default:
	}

	close(blocking.releaseCount)
	if err := <-done; err != nil {
		t.Fatalf("expected warmup success, got %v", err)
	}
}

func newWarmReliabilityService(
	t *testing.T,
	cat catalog.Catalog,
	stage Stage,
	catMetrics *catalog.RuntimeMetrics,
) *Service {
	t.Helper()
	asset := &catalog.Asset{
		Path:       "app.js",
		FullPath:   "app.js",
		MediaType:  "application/javascript",
		SourceHash: "source-hash",
	}
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}
	return newServiceState(
		&config.Compression{Enable: true, Mode: config.CompressionModeWarmup},
		slog.New(slog.DiscardHandler),
		cat,
		serviceDeps{
			stages:     cxlist.NewList(stage),
			obs:        observabilityx.Nop(),
			catMetrics: catMetrics,
		},
		0,
	)
}
