package server_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/arcgolabs/eventx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	appEvent "github.com/lyonbrown4d/spack/internal/event"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/server"
)

func TestPublishVariantServedPublishesEvent(t *testing.T) {
	bus := eventx.New()
	var received appEvent.VariantServed
	unsubscribe, err := eventx.Subscribe(bus, func(_ context.Context, event appEvent.VariantServed) error {
		received = event
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()

	server.PublishVariantServedForTest(context.Background(), &resolver.Result{
		Asset: &catalog.Asset{
			Path: "app.js",
		},
		Variant: &catalog.Variant{
			ArtifactPath: "/tmp/app.js.br",
		},
		FilePath:        "/tmp/app.js.br",
		MediaType:       "application/javascript",
		ContentEncoding: "br",
	}, bus, slog.New(slog.DiscardHandler))

	if received.ArtifactPath != "/tmp/app.js.br" {
		t.Fatalf("expected artifact path to be published, got %q", received.ArtifactPath)
	}
	if received.AssetPath != "app.js" {
		t.Fatalf("expected asset path to be published, got %q", received.AssetPath)
	}
	if received.ContentCoding != "br" {
		t.Fatalf("expected content coding br, got %q", received.ContentCoding)
	}
}
