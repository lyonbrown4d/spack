//go:build !spack_libvips

package pipeline

import (
	"errors"
	"log/slog"
	"testing"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
)

func TestWarmFailsForImageOnlyPipelineWhenEngineIsUnavailable(t *testing.T) {
	imageCfg := &config.Image{Enable: true}
	stage := newImageStage(
		imageCfg,
		newImageEngine(imageCfg, slog.New(slog.DiscardHandler), observabilityx.Nop()),
		nil,
		catalog.NewInMemoryCatalog(),
		nil,
	)
	svc := newServiceState(
		&config.Compression{Enable: false, Mode: config.CompressionModeOff},
		slog.New(slog.DiscardHandler),
		catalog.NewInMemoryCatalog(),
		serviceDeps{
			stages: cxlist.NewList[Stage](stage),
			obs:    observabilityx.Nop(),
		},
		0,
	)

	err := svc.Warm(t.Context())
	if err == nil {
		t.Fatal("expected image-only pipeline to reject unavailable image engine")
	}
	if errors.Is(err, ErrVariantSkipped) {
		t.Fatalf("expected configuration failure, got skip: %v", err)
	}
}
