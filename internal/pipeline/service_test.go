package pipeline_test

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/pipeline"
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

func TestEnqueueDropContractAllowsLaterRetry(t *testing.T) {
	svc := pipeline.NewServiceForTest(&config.Compression{
		Enable: true,
		Mode:   config.CompressionModeLazy,
	}, slog.New(slog.DiscardHandler), catalog.NewInMemoryCatalog(), 1)

	first := pipeline.Request{AssetPath: "a.js"}
	retryable := pipeline.Request{AssetPath: "b.js"}

	svc.Enqueue(first)
	svc.Enqueue(retryable)

	if pipeline.QueuedCountForTest(svc) != 1 {
		t.Fatalf("expected one queued request after full-queue drop, got %d", pipeline.QueuedCountForTest(svc))
	}
	if pipeline.PendingCountForTest(svc) != 1 {
		t.Fatalf("expected dropped request not to remain pending, got %d pending requests", pipeline.PendingCountForTest(svc))
	}

	drained := pipeline.DequeueRequestForTest(svc)
	if drained.AssetPath != first.AssetPath {
		t.Fatalf("expected queued request %q, got %q", first.AssetPath, drained.AssetPath)
	}
	pipeline.FinishRequestForTest(svc, drained)

	svc.Enqueue(retryable)
	if pipeline.QueuedCountForTest(svc) != 1 {
		t.Fatalf("expected retry request to be queued, got %d", pipeline.QueuedCountForTest(svc))
	}
	if pipeline.PendingCountForTest(svc) != 1 {
		t.Fatalf("expected retry request to become pending, got %d", pipeline.PendingCountForTest(svc))
	}
}
