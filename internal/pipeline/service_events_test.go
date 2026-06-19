package pipeline_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/arcgolabs/eventx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	appEvent "github.com/lyonbrown4d/spack/internal/event"
	"github.com/lyonbrown4d/spack/internal/pipeline"
)

func TestUpsertStageVariantPublishesGeneratedEvent(t *testing.T) {
	cat := catalog.NewInMemoryCatalog()
	asset := &catalog.Asset{
		Path:       "bundle.js",
		FullPath:   filepath.Join(t.TempDir(), "bundle.js"),
		MediaType:  "application/javascript",
		SourceHash: "hash-6",
		ETag:       "\"hash-6\"",
	}
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}

	bus := eventx.New()
	received := make(chan appEvent.VariantGenerated, 1)
	unsubscribe, err := eventx.Subscribe(bus, func(_ context.Context, event appEvent.VariantGenerated) error {
		select {
		case received <- event:
		default:
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()

	svc := pipeline.NewServiceWithBusForTest(&config.Compression{}, slog.New(slog.DiscardHandler), cat, bus, 1)
	pipeline.UpsertStageVariantForTest(svc, "compression", asset, &catalog.Variant{
		ID:           "bundle.js|encoding=br",
		AssetPath:    "bundle.js",
		ArtifactPath: "/tmp/bundle.js.br",
		Size:         128,
		MediaType:    "application/javascript",
		SourceHash:   "hash-6",
		ETag:         "\"hash-6-br\"",
		Encoding:     "br",
	})

	var event appEvent.VariantGenerated
	select {
	case event = <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("expected generated event")
	}
	if event.ArtifactPath != "/tmp/bundle.js.br" {
		t.Fatalf("expected generated event for artifact path, got %q", event.ArtifactPath)
	}
	if event.Stage != "compression" {
		t.Fatalf("expected generated event stage compression, got %q", event.Stage)
	}
	if event.Size != 128 {
		t.Fatalf("expected generated event size 128, got %d", event.Size)
	}
}
