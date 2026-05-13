package resolver

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	contentcodingspec "github.com/lyonbrown4d/spack/internal/contentcoding/spec"
	"github.com/lyonbrown4d/spack/internal/media"
	"log/slog"
)

// NewResolverForTest exposes resolver construction for external tests.
func NewResolverForTest(cfg *config.Assets, cat catalog.Catalog, logger *slog.Logger) *Resolver {
	return NewResolverWithObservabilityForTest(cfg, cat, logger, nil)
}

// NewResolverWithObservabilityForTest exposes resolver construction with an observability backend for external tests.
func NewResolverWithObservabilityForTest(
	cfg *config.Assets,
	cat catalog.Catalog,
	logger *slog.Logger,
	obs observabilityx.Observability,
) *Resolver {
	defaults := config.DefaultConfig()
	return newResolver(cfg, contentcoding.NewRegistry(contentcoding.Options{
		BrotliQuality: defaults.Compression.BrotliQuality,
		GzipLevel:     defaults.Compression.GzipLevel,
		ZstdLevel:     defaults.Compression.ZstdLevel,
	}, defaults.Compression.NormalizedEncodings()), cat, logger, obs)
}

// NewResolverWithCompressionForTest exposes resolver construction with compression config for external tests.
func NewResolverWithCompressionForTest(
	cfg *config.Assets,
	compression *config.Compression,
	cat catalog.Catalog,
	logger *slog.Logger,
) *Resolver {
	return newResolver(cfg, contentcoding.NewRegistry(contentcoding.Options{
		BrotliQuality: compression.BrotliQuality,
		GzipLevel:     compression.GzipLevel,
		ZstdLevel:     compression.ZstdLevel,
	}, compression.NormalizedEncodings()), cat, logger, nil)
}

// ParseAcceptEncodingForTest exposes encoding preference parsing for external tests.
func ParseAcceptEncodingForTest(header string) *cxlist.List[string] {
	return ParseAcceptEncoding(header, contentcodingspec.DefaultNames())
}

// ParseAcceptEncodingWithSupportedForTest exposes encoding preference parsing with a custom support list for external tests.
func ParseAcceptEncodingWithSupportedForTest(header string, supported *cxlist.List[string]) *cxlist.List[string] {
	return ParseAcceptEncoding(header, supported)
}

// ParseAcceptImageFormatsForTest exposes image format preference parsing for external tests.
func ParseAcceptImageFormatsForTest(header, sourceFormat string) *cxlist.List[string] {
	return ParseAcceptImageFormats(header, sourceFormat, media.SupportedImageFormats())
}
