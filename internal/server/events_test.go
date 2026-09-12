package server_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/arcgolabs/eventx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	appEvent "github.com/lyonbrown4d/spack/internal/event"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/server"
)

type unrelatedEvent struct{}

func (unrelatedEvent) Name() string {
	return "server.test.unrelated"
}

func TestPublishVariantServedPublishesEvent(t *testing.T) {
	bus := eventx.New()
	cleanupEventBus(t, bus)

	var received appEvent.VariantServed
	unsubscribe, err := bus.Subscribe(func(_ context.Context, event appEvent.VariantServed) error {
		received = event
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()

	beforePublish := time.Now()
	server.PublishVariantServedForTest(t.Context(), variantServedResult(), bus, slog.New(slog.DiscardHandler))
	afterPublish := time.Now()

	if received.ArtifactPath != "/tmp/app.js.br" {
		t.Fatalf("expected artifact path to be published, got %q", received.ArtifactPath)
	}
	if received.AssetPath != "app.js" {
		t.Fatalf("expected asset path to be published, got %q", received.AssetPath)
	}
	if received.ContentCoding != "br" {
		t.Fatalf("expected content coding br, got %q", received.ContentCoding)
	}
	if received.ContentType != "application/javascript" {
		t.Fatalf("expected content type application/javascript, got %q", received.ContentType)
	}
	if received.ServedAt.Before(beforePublish) || received.ServedAt.After(afterPublish) {
		t.Fatalf("expected served time between %v and %v, got %v", beforePublish, afterPublish, received.ServedAt)
	}
}

func TestPublishVariantServedLazySkipsFactoryWithoutSubscribers(t *testing.T) {
	bus := eventx.New()
	cleanupEventBus(t, bus)
	assertVariantServedFactoryCalls(t, bus, 0)
}

func TestPublishVariantServedLazySkipsFactoryWithUnrelatedSubscriber(t *testing.T) {
	bus := eventx.New()
	cleanupEventBus(t, bus)
	subscribeUnrelatedEvent(t, bus)
	assertVariantServedFactoryCalls(t, bus, 0)
}

func TestPublishVariantServedLazyCallsFactoryForTypedSubscriber(t *testing.T) {
	bus := eventx.New()
	cleanupEventBus(t, bus)
	subscribeVariantServed(t, bus)
	assertVariantServedFactoryCalls(t, bus, 1)
}

func TestPublishVariantServedNilLoggerDoesNotPanicOnPublishError(t *testing.T) {
	bus := eventx.New()
	cleanupEventBus(t, bus)
	unsubscribe, err := bus.Subscribe(func(_ context.Context, _ appEvent.VariantServed) error {
		return errors.New("subscriber failed")
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()

	server.PublishVariantServedForTest(t.Context(), variantServedResult(), bus, nil)
}

func BenchmarkPublishVariantServed(b *testing.B) {
	b.Run("no_subscribers", func(b *testing.B) {
		benchmarkPublishVariantServed(b, nil)
	})
	b.Run("unrelated_subscriber", func(b *testing.B) {
		benchmarkPublishVariantServed(b, subscribeUnrelatedEvent)
	})
	b.Run("variant_served_subscriber", func(b *testing.B) {
		benchmarkPublishVariantServed(b, subscribeVariantServed)
	})
}

func benchmarkPublishVariantServed(b *testing.B, subscribe func(testing.TB, *eventx.Bus)) {
	b.Helper()
	bus := eventx.New()
	cleanupEventBus(b, bus)
	if subscribe != nil {
		subscribe(b, bus)
	}
	ctx := context.Background()
	result := variantServedResult()

	b.ReportAllocs()
	for b.Loop() {
		server.PublishVariantServedForTest(ctx, result, bus, nil)
	}
}

func assertVariantServedFactoryCalls(t *testing.T, bus *eventx.Bus, want int) {
	t.Helper()
	calls := 0
	server.PublishVariantServedLazyForTest(t.Context(), bus, "app.js", nil, func() appEvent.VariantServed {
		calls++
		return appEvent.VariantServed{}
	})
	if calls != want {
		t.Fatalf("expected factory to be called %d times, got %d", want, calls)
	}
}

func cleanupEventBus(tb testing.TB, bus *eventx.Bus) {
	tb.Helper()
	tb.Cleanup(func() {
		if err := bus.Close(); err != nil {
			tb.Errorf("close event bus: %v", err)
		}
	})
}

func subscribeUnrelatedEvent(tb testing.TB, bus *eventx.Bus) {
	tb.Helper()
	unsubscribe, err := bus.Subscribe(func(_ context.Context, _ unrelatedEvent) error {
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(unsubscribe)
}

func subscribeVariantServed(tb testing.TB, bus *eventx.Bus) {
	tb.Helper()
	unsubscribe, err := bus.Subscribe(func(_ context.Context, _ appEvent.VariantServed) error {
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(unsubscribe)
}

func variantServedResult() *resolver.Result {
	return &resolver.Result{
		Asset: &catalog.Asset{
			Path: "app.js",
		},
		Variant: &catalog.Variant{
			ArtifactPath: "/tmp/app.js.br",
		},
		FilePath:        "/tmp/app.js.br",
		MediaType:       "application/javascript",
		ContentEncoding: "br",
	}
}
