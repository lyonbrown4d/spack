//go:build !spack_libvips

package pipeline

import (
	"log/slog"
	"testing"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
)

func TestWarmFailsWhenConfiguredImageEngineIsUnavailable(t *testing.T) {
	imageCfg := &config.Image{Enable: true}
	stage := newImageStage(
		imageCfg,
		newImageEngine(imageCfg, slog.New(slog.DiscardHandler), observabilityx.Nop()),
		nil,
		catalog.NewInMemoryCatalog(),
		nil,
	)
	svc := newServiceState(
		&config.Compression{Enable: true, Mode: config.CompressionModeWarmup},
		slog.New(slog.DiscardHandler),
		catalog.NewInMemoryCatalog(),
		serviceDeps{
			stages: cxlist.NewList[Stage](stage),
			obs:    observabilityx.Nop(),
		},
		0,
	)

	if err := svc.Warm(t.Context()); err == nil {
		t.Fatal("expected unavailable image engine to fail compiler warmup")
	}
}
