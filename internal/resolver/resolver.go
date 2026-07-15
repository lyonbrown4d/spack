package resolver

import (
	"context"
	"errors"
	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	contentcodingspec "github.com/lyonbrown4d/spack/internal/contentcoding/spec"
	"github.com/lyonbrown4d/spack/internal/media"
	"github.com/lyonbrown4d/spack/internal/requestpath"
	"github.com/samber/oops"
	"log/slog"
	"path"
	"strings"
	"time"
)

var (
	ErrNotFound           = errors.New("asset not found")
	errResolverContextNil = errors.New("resolver context is nil")

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
		supportedEncodings: newEncodingSupportFromValues(contentcodingspec.NormalizeNames(registry.Names())),
		catalog:            cat,
		logger:             logger,
		obs:                obs,
	}
}

func (r *Resolver) Resolve(ctx context.Context, request Request) (*Result, error) {
	var ctxErr error
	ctx, ctxErr = requireResolveContext(ctx)
	if ctxErr != nil {
		return nil, ctxErr
	}
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

func requireResolveContext(ctx context.Context) (context.Context, error) {
	if ctx == nil {
		return nil, oops.In("resolver").Owner("resolve").Wrap(errResolverContextNil)
	}
	return ctx, nil
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
