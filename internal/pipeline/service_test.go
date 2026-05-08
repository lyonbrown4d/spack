package pipeline_test

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/daiyuang/spack/internal/catalog"
	"github.com/daiyuang/spack/internal/config"
	"github.com/daiyuang/spack/internal/pipeline"
	"log/slog"
	"testing"
)

func TestEnqueueDeduplicatesRequests(t *testing.T) {
	svc := pipeline.NewServiceForTest(&config.Compression{
		Enable: true,
		Mode:   config.CompressionModeLazy,
	}, slog.New(slog.DiscardHandler), catalog.NewInMemoryCatalog(), 2)

	svc.Enqueue(pipeline.Request{
		AssetPath:          "hero.png",
		PreferredEncodings: cxlist.NewList("gzip", "br"),
		PreferredFormats:   cxlist.NewList("jpeg", "png"),
		PreferredWidths:    cxlist.NewList(1280, 640),
	})
	svc.Enqueue(pipeline.Request{
		AssetPath:          "hero.png",
		PreferredEncodings: cxlist.NewList("br", "gzip"),
		PreferredFormats:   cxlist.NewList("png", "jpeg"),
		PreferredWidths:    cxlist.NewList(640, 1280),
	})

	if pipeline.QueuedCountForTest(svc) != 1 {
		t.Fatalf("expected one queued request, got %d", pipeline.QueuedCountForTest(svc))
	}
	if pipeline.PendingCountForTest(svc) != 1 {
		t.Fatalf("expected one pending request, got %d", pipeline.PendingCountForTest(svc))
	}
}

func TestEnqueueDropsWhenQueueFull(t *testing.T) {
	svc := pipeline.NewServiceForTest(&config.Compression{
		Enable: true,
		Mode:   config.CompressionModeLazy,
	}, slog.New(slog.DiscardHandler), catalog.NewInMemoryCatalog(), 1)

	svc.Enqueue(pipeline.Request{AssetPath: "a.js"})
	svc.Enqueue(pipeline.Request{AssetPath: "b.js"})

	if pipeline.QueuedCountForTest(svc) != 1 {
		t.Fatalf("expected one queued request, got %d", pipeline.QueuedCountForTest(svc))
	}
	if pipeline.PendingCountForTest(svc) != 1 {
		t.Fatalf("expected one pending request, got %d", pipeline.PendingCountForTest(svc))
	}
}
