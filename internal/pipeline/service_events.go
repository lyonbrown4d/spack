package pipeline

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/arcgolabs/eventx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	appEvent "github.com/lyonbrown4d/spack/internal/event"
	"github.com/samber/oops"
)

func (s *Service) subscribeVariantServed() error {
	if s == nil || s.bus == nil || s.variantServedUnsubscribe != nil {
		return nil
	}

	unsubscribe, err := eventx.Subscribe(s.bus, func(_ context.Context, event appEvent.VariantServed) error {
		s.markVariantHitAt(event.ArtifactPath, event.ServedAt)
		return nil
	})
	if err != nil {
		return oops.In("pipeline").Owner("events").Wrap(oops.Wrapf(err, "subscribe variant served"))
	}
	s.variantServedUnsubscribe = unsubscribe
	return nil
}

func (s *Service) unsubscribeVariantServed() {
	if s == nil || s.variantServedUnsubscribe == nil {
		return
	}
	s.variantServedUnsubscribe()
	s.variantServedUnsubscribe = nil
}

func (s *Service) publishVariantRemoved(ctx context.Context, path string, reason appEvent.VariantRemovalReason) {
	if !s.canPublishArtifactEvent(path) {
		return
	}

	err := s.bus.Publish(ctx, appEvent.VariantRemoved{
		ArtifactPath: path,
		Reason:       reason,
		RemovedAt:    time.Now(),
	})
	if shouldIgnoreBusPublishError(err) {
		return
	}

	s.logger.Debug("Publish variant removed event failed",
		slog.String("path", path),
		slog.String("reason", string(reason)),
		slog.String("err", err.Error()),
	)
}

func (s *Service) publishVariantGenerated(ctx context.Context, stage string, variant *catalog.Variant) {
	if s == nil || variant == nil || !s.canPublishArtifactEvent(variant.ArtifactPath) {
		return
	}

	event := appEvent.VariantGenerated{
		AssetPath:    variant.AssetPath,
		ArtifactPath: variant.ArtifactPath,
		Stage:        strings.TrimSpace(stage),
		Size:         variant.Size,
		GeneratedAt:  time.Now(),
	}
	if err := s.bus.PublishAsync(ctx, event); err != nil {
		if errors.Is(err, eventx.ErrAsyncRuntimeUnavailable) {
			s.publishVariantGeneratedSync(ctx, event)
			return
		}
		if shouldIgnoreBusPublishError(err) {
			return
		}
		s.logger.Debug("Publish variant generated event failed",
			slog.String("path", variant.ArtifactPath),
			slog.String("stage", stage),
			slog.String("err", err.Error()),
		)
	}
}

func (s *Service) publishVariantGeneratedSync(ctx context.Context, event appEvent.VariantGenerated) {
	err := s.bus.Publish(ctx, event)
	if shouldIgnoreBusPublishError(err) {
		return
	}

	s.logger.Debug("Publish variant generated event sync fallback failed",
		slog.String("path", event.ArtifactPath),
		slog.String("stage", event.Stage),
		slog.String("err", err.Error()),
	)
}

func (s *Service) canPublishArtifactEvent(path string) bool {
	return s != nil && s.bus != nil && strings.TrimSpace(path) != ""
}

func shouldIgnoreBusPublishError(err error) bool {
	return err == nil || errors.Is(err, eventx.ErrBusClosed)
}
