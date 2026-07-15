package pipeline_test

import (
	"fmt"
	"log/slog"
	"testing"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/pipeline"
)

func BenchmarkServiceEnqueueUnique(b *testing.B) {
	svc := pipeline.NewServiceForTest(&config.Compression{
		Enable: true,
		Mode:   config.CompressionModeLazy,
	}, slog.New(slog.DiscardHandler), catalog.NewInMemoryCatalog(), 1)

	requests := make([]pipeline.Request, 1024)
	for i := range requests {
		requests[i] = pipeline.Request{
			AssetPath:          fmt.Sprintf("asset-%d.js", i),
			PreferredEncodings: cxlist.NewList("br", "gzip"),
			PreferredFormats:   cxlist.NewList("jpeg", "png"),
			PreferredWidths:    cxlist.NewList(640, 1280),
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	i := 0
	for b.Loop() {
		req := requests[i%len(requests)]
		svc.Enqueue(req)

		queued := pipeline.DequeueRequestForTest(svc)
		pipeline.FinishRequestForTest(svc, queued)
		i++
	}
}

func BenchmarkServiceEnqueueDeduplicated(b *testing.B) {
	svc := pipeline.NewServiceForTest(&config.Compression{
		Enable: true,
		Mode:   config.CompressionModeLazy,
	}, slog.New(slog.DiscardHandler), catalog.NewInMemoryCatalog(), 1)

	req := pipeline.Request{
		AssetPath:          "hero.png",
		PreferredEncodings: cxlist.NewList("br", "gzip"),
		PreferredFormats:   cxlist.NewList("jpeg", "png"),
		PreferredWidths:    cxlist.NewList(640, 1280),
	}
	svc.Enqueue(req)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		svc.Enqueue(req)
	}
}
