package resolver

import (
	"context"
	"errors"
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/observabilityx"
	"github.com/daiyuang/spack/internal/catalog"
	"github.com/daiyuang/spack/internal/config"
	"github.com/daiyuang/spack/internal/contentcoding"
	contentcodingspec "github.com/daiyuang/spack/internal/contentcoding/spec"
	"github.com/daiyuang/spack/internal/media"
	"github.com/daiyuang/spack/internal/requestpath"
	"log/slog"
	"path"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("asset not found")

	resolverResolutionsTotalSpec = observabilityx.NewCounterSpec(
		"resolver_resolutions_total",
		observabilityx.WithDescription("Total number of asset resolution attempts."),
		observabilityx.WithLabelKeys("result"),
	)
	resolverResolutionDurationSpec = observabilityx.NewHistogramSpec(
		"resolver_resolution_duration_seconds",
		observabilityx.WithDescription("Asset resolution duration in seconds."),
		observabilityx.WithUnit("s"),
		observabilityx.WithLabelKeys("result"),
	)
	resolverGenerationRequestsTotalSpec = observabilityx.NewCounterSpec(
		"resolver_generation_requests_total",
		observabilityx.WithDescription("Total number of requested generated artifact dimensions by kind."),
		observabilityx.WithLabelKeys("kind"),
	)
)

func newResolver(
	cfg *config.Assets,
	registry contentcoding.Registry,
	cat catalog.Catalog,
	logger *slog.Logger,
	obs observabilityx.Observability,
) *Resolver {
	if obs != nil {
		obs = observabilityx.Normalize(obs, logger)
	}
	return &Resolver{
		cfg:                cfg,
		supportedEncodings: contentcodingspec.NormalizeNames(registry.Names()),
		catalog:            cat,
		logger:             logger,
		obs:                obs,
	}
}

func (r *Resolver) Resolve(ctx context.Context, request Request) (*Result, error) {
	ctx = normalizeResolveContext(ctx)
	startedAt := time.Now()
	asset, fallbackUsed, err := r.findAsset(request.Path)
	if err != nil {
		r.recordMetrics(ctx, startedAt, nil, err)
		return nil, err
	}
	if asset == nil {
		r.recordMetrics(ctx, startedAt, nil, ErrNotFound)
		return nil, ErrNotFound
	}

	requestedFormat := media.NormalizeImageFormat(request.Format)
	request.Format = requestedFormat

	requestedImageFormats := request.PreferredFormats
	if requestedImageFormats == nil {
		requestedImageFormats = preferredImageFormats(request.Accept, requestedFormat, asset.MediaType)
	}
	if result, ok, err := r.resolvePreferredVariant(
		ctx,
		startedAt,
		asset,
		fallbackUsed,
		&request,
		requestedImageFormats,
	); err != nil || ok {
		return result, err
	}

	result := &Result{
		Asset:              asset,
		FilePath:           asset.FullPath,
		MediaType:          asset.MediaType,
		ETag:               asset.ETag,
		PreferredEncodings: request.PreferredEncodings,
		PreferredWidths:    preferredWidths(request.Width),
		PreferredFormats:   requestedImageFormats,
		FallbackUsed:       fallbackUsed,
	}
	r.recordMetrics(ctx, startedAt, result, nil)
	return result, nil
}

func (r *Resolver) resolvePreferredVariant(
	ctx context.Context,
	startedAt time.Time,
	asset *catalog.Asset,
	fallbackUsed bool,
	request *Request,
	preferredImageFormats *cxlist.List[string],
) (*Result, bool, error) {
	if request.Width > 0 || preferredImageFormats.Len() > 0 {
		result, ok, err := r.resolveImageVariant(ctx, startedAt, asset, fallbackUsed, request.Width, preferredImageFormats)
		if ok || err != nil {
			return result, ok, err
		}
	}
	encodings := request.PreferredEncodings
	if encodings == nil {
		encodings = parseAcceptEncoding(request.AcceptEncoding, r.supportedEncodings)
	}
	if request.RangeRequested || encodings.Len() == 0 {
		return nil, false, nil
	}
	request.PreferredEncodings = encodings
	return r.resolveEncodingVariant(ctx, startedAt, asset, fallbackUsed, encodings)
}

func (r *Resolver) resolveImageVariant(
	ctx context.Context,
	startedAt time.Time,
	asset *catalog.Asset,
	fallbackUsed bool,
	width int,
	formats *cxlist.List[string],
) (*Result, bool, error) {
	variant, err := r.pickImageVariant(asset, width, formats)
	if err != nil {
		r.recordMetrics(ctx, startedAt, nil, err)
		return nil, false, err
	}
	if variant == nil {
		return nil, false, nil
	}
	result := &Result{
		Asset:        asset,
		Variant:      variant,
		FilePath:     variant.ArtifactPath,
		MediaType:    firstNonEmpty(variant.MediaType, asset.MediaType),
		ETag:         firstNonEmpty(variant.ETag, asset.ETag),
		FallbackUsed: fallbackUsed,
	}
	r.recordMetrics(ctx, startedAt, result, nil)
	return result, true, nil
}

func (r *Resolver) resolveEncodingVariant(
	ctx context.Context,
	startedAt time.Time,
	asset *catalog.Asset,
	fallbackUsed bool,
	encodings *cxlist.List[string],
) (*Result, bool, error) {
	variant, err := r.pickVariant(asset, encodings)
	if err != nil {
		r.recordMetrics(ctx, startedAt, nil, err)
		return nil, false, err
	}
	if variant == nil {
		return nil, false, nil
	}
	result := &Result{
		Asset:           asset,
		Variant:         variant,
		FilePath:        variant.ArtifactPath,
		MediaType:       asset.MediaType,
		ContentEncoding: variant.Encoding,
		ETag:            firstNonEmpty(variant.ETag, asset.ETag),
		FallbackUsed:    fallbackUsed,
	}
	r.recordMetrics(ctx, startedAt, result, nil)
	return result, true, nil
}

func (r *Resolver) ResolveAfterVariantArtifactMiss(ctx context.Context, request Request, variant *catalog.Variant) (*Result, error) {
	if r == nil {
		return nil, ErrNotFound
	}
	if variant != nil {
		r.catalog.DeleteVariantByArtifactPath(variant.ArtifactPath)
	}
	return r.Resolve(ctx, request)
}

func (r *Resolver) recordMetrics(ctx context.Context, startedAt time.Time, result *Result, err error) {
	if r == nil || r.obs == nil {
		return
	}

	attrs := []observabilityx.Attribute{
		observabilityx.String("result", resolutionResultKind(result, err)),
	}
	r.obs.Counter(resolverResolutionsTotalSpec).Add(ctx, 1, attrs...)
	r.obs.Histogram(resolverResolutionDurationSpec).Record(ctx, time.Since(startedAt).Seconds(), attrs...)

	if result == nil {
		return
	}

	if count := int64(result.PreferredEncodings.Len()); count > 0 {
		r.obs.Counter(resolverGenerationRequestsTotalSpec).Add(ctx, count,
			observabilityx.String("kind", "encoding"),
		)
	}
	if count := int64(result.PreferredWidths.Len()); count > 0 {
		r.obs.Counter(resolverGenerationRequestsTotalSpec).Add(ctx, count,
			observabilityx.String("kind", "image_width"),
		)
	}
	if count := int64(result.PreferredFormats.Len()); count > 0 {
		r.obs.Counter(resolverGenerationRequestsTotalSpec).Add(ctx, count,
			observabilityx.String("kind", "image_format"),
		)
	}
}

func normalizeResolveContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func resolutionResultKind(result *Result, err error) string {
	if errors.Is(err, ErrNotFound) {
		return "not_found"
	}
	if err != nil {
		return "error"
	}
	if result == nil {
		return "empty"
	}
	if kind, ok := resolutionVariantKind(result.Variant); ok {
		return kind
	}
	if result.FallbackUsed {
		return "fallback_asset"
	}
	return "asset"
}

func resolutionVariantKind(variant *catalog.Variant) (string, bool) {
	if variant == nil {
		return "", false
	}
	if variant.Width > 0 || strings.TrimSpace(variant.Format) != "" {
		return "image_variant", true
	}
	if strings.TrimSpace(variant.Encoding) != "" {
		return "encoding_variant", true
	}
	return "variant", true
}

func (r *Resolver) findAsset(requestPath string) (*catalog.Asset, bool, error) {
	resolvedPath := requestpath.Clean(requestPath)
	if asset, ok, err := r.findPrimaryAsset(resolvedPath); ok || err != nil {
		return asset, false, err
	}

	if r.cfg.Fallback.On == config.FallbackOnNotFound && resolvedPath.AllowsEntryFallback {
		target := requestpath.Clean(r.cfg.Fallback.Target).Value
		if target != "" {
			asset, ok, err := findAssetForRead(r.catalog, target)
			if ok || err != nil {
				return asset, true, err
			}
		}
	}
	return nil, false, nil
}

func (r *Resolver) findPrimaryAsset(requestPath requestpath.Cleaned) (*catalog.Asset, bool, error) {
	if requestPath.Value == "" {
		return findAssetForRead(r.catalog, r.cfg.Entry)
	}

	if asset, ok, err := findAssetForRead(r.catalog, requestPath.Value); ok || err != nil {
		return asset, ok, err
	}
	if !requestPath.AllowsEntryFallback {
		return nil, false, nil
	}

	candidate := path.Join(requestPath.Value, r.cfg.Entry)
	if candidate == requestPath.Value {
		return nil, false, nil
	}
	return findAssetForRead(r.catalog, candidate)
}
