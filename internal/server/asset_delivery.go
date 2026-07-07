package server

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/cachepolicy"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/lo"
)

func (r *assetDeliveryRuntime) sendResolvedAsset(
	c fiber.Ctx,
	request resolver.Request,
	result *resolver.Result,
	requestedFormat string,
) (string, error) {
	cacheRequest := buildMemoryCacheRequest(result, request)
	if r.bodyCache.ShouldServeRequest(cacheRequest) {
		if delivery, ok, err := r.trySendHotResolvedAsset(c, request, result, requestedFormat); ok || err != nil {
			return delivery, err
		}
	}

	headerPlan := newResolvedHeaderPlan(r.responsePolicy, result, requestedFormat)
	headerPlan.ApplyForRange(c, request.RangeRequested)
	if handled := handleConditionalAssetRequest(c, request); handled {
		return "", nil
	}
	r.applyResourceHints(c, request, result)

	if r.bodyCache.ShouldServeRequest(cacheRequest) {
		return r.sendCachedResolvedAsset(c, result, cacheRequest, hotResponseCacheKey(result, requestedFormat), headerPlan)
	}

	return r.sendResolvedAssetFile(c, request, result, headerPlan)
}

func (r *assetDeliveryRuntime) trySendHotResolvedAsset(
	c fiber.Ctx,
	request resolver.Request,
	result *resolver.Result,
	requestedFormat string,
) (string, bool, error) {
	if !shouldUseHotResponse(c, request, result) {
		return "", false, nil
	}

	entry, found := r.bodyCache.GetCachedEntry(result.FilePath)
	if !found {
		return "", false, nil
	}
	hot, ok := entry.Attachment.(*hotResponseEntry)
	if !ok || hot == nil || hot.Key != hotResponseCacheKey(result, requestedFormat) {
		return "", false, nil
	}

	hot.HeaderPlan.Apply(c)
	if handled := handleConditionalAssetRequest(c, request); handled {
		return "", true, nil
	}
	r.applyResourceHints(c, request, result)
	if err := c.Send(entry.Body); err != nil {
		return "", true, fmt.Errorf("send hot cached asset body: %w", err)
	}
	return deliveryHotResponseHit, true, nil
}

func (r *assetDeliveryRuntime) applyResourceHints(c fiber.Ctx, request resolver.Request, result *resolver.Result) {
	if !r.canApplyResourceHints(c, request) {
		return
	}

	entry, ok := r.resourceHints.Entry(result)
	if !ok || entry.links == nil || entry.links.IsEmpty() {
		return
	}
	r.sendResourceHintEarlyHints(c, entry.links)
	if entry.header != "" {
		c.Set(fiber.HeaderLink, entry.header)
	}
}

func (r *assetDeliveryRuntime) canApplyResourceHints(c fiber.Ctx, request resolver.Request) bool {
	return r != nil &&
		r.resourceHints != nil &&
		c.Method() == fiber.MethodGet &&
		!request.RangeRequested
}

func (r *assetDeliveryRuntime) sendResourceHintEarlyHints(c fiber.Ctx, links resourceHintLinks) {
	if !r.resourceHints.EarlyHintsEnabled() {
		return
	}
	if err := r.sendEarlyResourceHints(c, links); err != nil && r.logger != nil {
		r.logger.Debug("Send early resource hints failed", slog.String("err", err.Error()))
	}
}

func handleConditionalAssetRequest(c fiber.Ctx, request resolver.Request) bool {
	if shouldSendNotModified(c, request) {
		c.Status(fiber.StatusNotModified)
		return true
	}
	if c.Method() == fiber.MethodHead {
		c.Status(fiber.StatusOK)
		return true
	}
	return false
}

func (r *assetDeliveryRuntime) sendCachedResolvedAsset(
	c fiber.Ctx,
	result *resolver.Result,
	request cachepolicy.MemoryRequest,
	hotKey hotResponseKey,
	headerPlan resolvedHeaderPlan,
) (string, error) {
	entry, found, err := r.bodyCache.GetEntryForServe(
		result.FilePath,
		request,
		&hotResponseEntry{Key: hotKey, HeaderPlan: headerPlan},
		hotResponseMatches(hotKey),
	)
	if err != nil {
		if missingErr := newMissingResolvedVariantError(result, err); missingErr != nil {
			return "", missingErr
		}
		return "", fiber.ErrInternalServerError
	}
	return sendCachedResolvedAssetEntry(c, entry, found)
}

func sendCachedResolvedAssetEntry(c fiber.Ctx, entry assetcache.Entry, found bool) (string, error) {
	if err := c.Send(entry.Body); err != nil {
		return "", fmt.Errorf("send cached asset body: %w", err)
	}
	return lo.Ternary(found, deliveryMemoryCacheHit, deliveryMemoryCacheFill), nil
}

func (r *assetDeliveryRuntime) sendResolvedAssetFile(
	c fiber.Ctx,
	request resolver.Request,
	result *resolver.Result,
	headerPlan resolvedHeaderPlan,
) (string, error) {
	if spackbundle.IsReference(result.FilePath) {
		return r.sendResolvedBundleAsset(c, request, result, headerPlan)
	}
	if request.RangeRequested {
		return r.sendResolvedAssetFileRange(c, result, headerPlan)
	}
	if err := c.SendFile(result.FilePath, fiber.SendFile{ByteRange: true}); err != nil {
		if missingErr := newMissingResolvedVariantError(result, err); missingErr != nil {
			return "", missingErr
		}
		if r.logger != nil {
			r.logger.Error("Send asset failed",
				slog.String("path", result.FilePath),
				slog.String("err", err.Error()),
			)
		}
		return "", fiber.ErrInternalServerError
	}

	// Override Fiber's extension-derived headers so variant metadata stays authoritative.
	headerPlan.ApplySendFileOverrides(c, request.RangeRequested)
	if request.RangeRequested {
		return deliverySendFileRange, nil
	}
	return deliverySendFile, nil
}

func resolvedAssetSize(result *resolver.Result) (int64, bool) {
	if result == nil {
		return 0, false
	}
	if size, ok := variantSize(result.Variant); ok {
		return size, true
	}
	return assetSize(result.Asset)
}

func buildMemoryCacheRequest(result *resolver.Result, request resolver.Request) cachepolicy.MemoryRequest {
	if result == nil {
		return cachepolicy.MemoryRequest{
			RangeRequested: request.RangeRequested,
			UseCase:        cachepolicy.MemoryUseCaseServe,
		}
	}

	cacheRequest := cachepolicy.MemoryRequest{
		Path:           result.FilePath,
		MediaType:      result.MediaType,
		RangeRequested: request.RangeRequested,
		UseCase:        cachepolicy.MemoryUseCaseServe,
		Kind:           cachepolicy.MemoryEntryKindAsset,
	}

	if result.Asset != nil {
		cacheRequest.AssetPath = result.Asset.Path
		cacheRequest.Size = result.Asset.Size
		cacheRequest.MediaType = result.Asset.MediaType
	}

	if result.Variant != nil {
		cacheRequest.AssetPath = result.Variant.AssetPath
		cacheRequest.Size = result.Variant.Size
		if mediaType := strings.TrimSpace(result.Variant.MediaType); mediaType != "" {
			cacheRequest.MediaType = mediaType
		}
		cacheRequest.Encoding = result.Variant.Encoding
		cacheRequest.Format = result.Variant.Format
		cacheRequest.Width = result.Variant.Width
		cacheRequest.Kind = cachepolicy.MemoryEntryKindVariant
	}

	return cacheRequest
}

type hotResponseEntry struct {
	Key        hotResponseKey
	HeaderPlan resolvedHeaderPlan
}

type hotResponseKey struct {
	filePath        string
	mediaType       string
	contentEncoding string
	etag            string
	size            int64
	requestedFormat string
}

func hotResponseMatches(key hotResponseKey) func(any) bool {
	return func(attachment any) bool {
		hot, ok := attachment.(*hotResponseEntry)
		return ok && hot != nil && hot.Key == key
	}
}

func shouldUseHotResponse(c fiber.Ctx, request resolver.Request, result *resolver.Result) bool {
	return result != nil &&
		strings.TrimSpace(result.FilePath) != "" &&
		c.Method() == fiber.MethodGet &&
		!request.RangeRequested
}

func hotResponseCacheKey(result *resolver.Result, requestedFormat string) hotResponseKey {
	if result == nil {
		return hotResponseKey{}
	}

	size, _ := resolvedAssetSize(result)
	return hotResponseKey{
		filePath:        result.FilePath,
		mediaType:       result.MediaType,
		contentEncoding: result.ContentEncoding,
		etag:            result.ETag,
		size:            size,
		requestedFormat: requestedFormat,
	}
}

type missingResolvedVariantError struct {
	artifactPath string
	cause        error
}

func (e *missingResolvedVariantError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("missing resolved variant artifact %q: %v", e.artifactPath, e.cause)
}

func (e *missingResolvedVariantError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func newMissingResolvedVariantError(result *resolver.Result, err error) error {
	if result == nil || result.Variant == nil || err == nil || !isMissingServerAsset(err) {
		return nil
	}
	return &missingResolvedVariantError{
		artifactPath: result.Variant.ArtifactPath,
		cause:        err,
	}
}
