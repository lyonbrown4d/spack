package resolver

import (
	"context"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/daiyuang/spack/internal/catalog"
)

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
