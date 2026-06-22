package assetcache

import (
	"context"
	"log/slog"

	"github.com/arcgolabs/eventx"
	"github.com/lyonbrown4d/spack/internal/cachepolicy"
	appEvent "github.com/lyonbrown4d/spack/internal/event"
	"github.com/samber/oops"
)

func (c *Cache) start(_ context.Context) error {
	if c == nil || c.bus == nil || !c.Enabled() {
		return nil
	}
	if c.variantRemovedUnsubscribe != nil || c.variantGeneratedUnsubscribe != nil {
		return nil
	}

	unsubscribe, err := c.subscribeVariantRemoved()
	if err != nil {
		return err
	}
	c.variantRemovedUnsubscribe = unsubscribe

	generatedUnsubscribe, err := c.subscribeVariantGenerated()
	if err != nil {
		unsubscribe()
		c.variantRemovedUnsubscribe = nil
		return err
	}
	c.variantGeneratedUnsubscribe = generatedUnsubscribe
	return nil
}

func (c *Cache) stop(_ context.Context) error {
	if c == nil {
		return nil
	}
	unsubscribeAll(c.variantRemovedUnsubscribe, c.variantGeneratedUnsubscribe)
	c.variantRemovedUnsubscribe = nil
	c.variantGeneratedUnsubscribe = nil
	if c.cache != nil {
		c.cache.Close()
		c.cache = nil
	}
	return nil
}

func (c *Cache) subscribeVariantRemoved() (func(), error) {
	unsubscribe, err := eventx.Subscribe(c.bus, func(_ context.Context, event appEvent.VariantRemoved) error {
		c.Delete(event.ArtifactPath)
		return nil
	})
	if err != nil {
		return nil, oops.Wrapf(err, "subscribe variant removed")
	}
	return unsubscribe, nil
}

func (c *Cache) subscribeVariantGenerated() (func(), error) {
	unsubscribe, err := eventx.Subscribe(c.bus, func(_ context.Context, event appEvent.VariantGenerated) error {
		if preloadErr := c.preloadPath(event.ArtifactPath, cachepolicy.MemoryRequest{
			Path:      event.ArtifactPath,
			AssetPath: event.AssetPath,
			Size:      event.Size,
			Kind:      cachepolicy.MemoryEntryKindVariant,
			UseCase:   cachepolicy.MemoryUseCaseEvent,
		}, nil); preloadErr != nil && c.logger != nil {
			c.logger.Debug("Preload generated variant failed",
				slog.String("path", event.ArtifactPath),
				slog.String("stage", event.Stage),
				slog.String("err", preloadErr.Error()),
			)
		}
		return nil
	})
	if err != nil {
		return nil, oops.Wrapf(err, "subscribe variant generated")
	}
	return unsubscribe, nil
}

func unsubscribeAll(unsubscribes ...func()) {
	for _, unsubscribe := range unsubscribes {
		if unsubscribe != nil {
			unsubscribe()
		}
	}
}
