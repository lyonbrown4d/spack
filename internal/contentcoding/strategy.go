package contentcoding

import (
	"bytes"
	"compress/gzip"
	"fmt"

	"github.com/andybalholm/brotli"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/klauspost/compress/zstd"
	"github.com/lyonbrown4d/spack/internal/contentcoding/spec"
)

type Options struct {
	BrotliQuality int
	GzipLevel     int
	ZstdLevel     int
}

type Strategy interface {
	Name() string
	Suffix() string
	Compress(raw []byte) ([]byte, error)
}

type Registry struct {
	strategies *cxmapping.Map[string, Strategy]
	names      *cxlist.List[string]
}

func NewRegistry(opts Options, enabled *cxlist.List[string]) Registry {
	all := cxmapping.AssociateList[Strategy, string, Strategy](newBuiltinStrategies(opts), func(_ int, strategy Strategy) (string, Strategy) {
		return strategy.Name(), strategy
	})

	enabled = spec.NormalizeNames(enabled)
	if enabled.IsEmpty() {
		enabled = spec.DefaultNames()
	}

	strategies := cxmapping.NewMapWithCapacity[string, Strategy](enabled.Len())
	enabled.Range(func(_ int, name string) bool {
		if strategy, ok := all.Get(name); ok {
			strategies.Set(name, strategy)
		}
		return true
	})
	return Registry{strategies: strategies, names: enabled}
}

func (r Registry) Lookup(name string) (Strategy, bool) {
	return r.strategies.Get(name)
}

func (r Registry) Names() *cxlist.List[string] {
	if r.names.IsEmpty() {
		return spec.DefaultNames()
	}
	return r.names
}

func newBuiltinStrategies(opts Options) *cxlist.List[Strategy] {
	return cxlist.NewList[Strategy](
		NewBrotliStrategy(opts.BrotliQuality),
		NewZstdStrategy(opts.ZstdLevel),
		NewGzipStrategy(opts.GzipLevel),
	)
}

type BrotliStrategy struct {
	quality int
}

func NewBrotliStrategy(quality int) Strategy {
	return BrotliStrategy{quality: quality}
}

func (s BrotliStrategy) Name() string {
	return "br"
}

func (s BrotliStrategy) Suffix() string {
	return ".br"
}

func (s BrotliStrategy) Compress(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := brotli.NewWriterLevel(&buf, clampBrotliQuality(s.quality))
	if _, err := writer.Write(raw); err != nil {
		return nil, fmt.Errorf("write brotli payload: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close brotli writer: %w", err)
	}
	return buf.Bytes(), nil
}

type ZstdStrategy struct {
	level int
}

func NewZstdStrategy(level int) Strategy {
	return ZstdStrategy{level: level}
}

func (s ZstdStrategy) Name() string {
	return "zstd"
}

func (s ZstdStrategy) Suffix() string {
	return ".zst"
}

func (s ZstdStrategy) Compress(raw []byte) ([]byte, error) {
	encoder, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(clampZstdLevel(s.level))),
		zstd.WithEncoderConcurrency(1),
	)
	if err != nil {
		return nil, fmt.Errorf("create zstd encoder: %w", err)
	}
	compressed := encoder.EncodeAll(raw, nil)
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close zstd encoder: %w", err)
	}

	return compressed, nil
}

type GzipStrategy struct {
	level int
}

func NewGzipStrategy(level int) Strategy {
	return GzipStrategy{level: level}
}

func (s GzipStrategy) Name() string {
	return "gzip"
}

func (s GzipStrategy) Suffix() string {
	return ".gz"
}

func (s GzipStrategy) Compress(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, clampGzipLevel(s.level))
	if err != nil {
		return nil, fmt.Errorf("create gzip writer: %w", err)
	}
	if _, err := writer.Write(raw); err != nil {
		return nil, fmt.Errorf("write gzip payload: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close gzip writer: %w", err)
	}
	return buf.Bytes(), nil
}

func clampGzipLevel(level int) int {
	if level < gzip.BestSpeed || level > gzip.BestCompression {
		return gzip.DefaultCompression
	}
	return level
}

func clampBrotliQuality(q int) int {
	if q < 0 {
		return 0
	}
	if q > 11 {
		return 11
	}
	return q
}

func clampZstdLevel(level int) int {
	if level < 0 {
		return 0
	}
	if level > 22 {
		return 22
	}
	return level
}
