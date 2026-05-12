package server

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/cachepolicy"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/samber/lo"
)

func (r *assetDeliveryRuntime) sendResolvedAsset(
	c fiber.Ctx,
	request resolver.Request,
	result *resolver.Result,
	requestedFormat string,
) (string, error) {
	headerPlan := newResolvedHeaderPlan(r.responsePolicy, result, requestedFormat)
	headerPlan.Apply(c)
	if handled := handleConditionalAssetRequest(c, request); handled {
		return "", nil
	}
	r.applyResourceHints(c, request, result)

	cacheRequest := buildMemoryCacheRequest(result, request)
	if r.bodyCache.ShouldServeRequest(cacheRequest) {
		return r.sendCachedResolvedAsset(c, result, cacheRequest)
	}

	return r.sendResolvedAssetFile(c, request, result, headerPlan)
}

func (r *assetDeliveryRuntime) applyResourceHints(c fiber.Ctx, request resolver.Request, result *resolver.Result) {
	if r == nil || r.resourceHints == nil || c.Method() != fiber.MethodGet || request.RangeRequested {
		return
	}

	links := r.resourceHints.Links(result)
	if links == nil || links.IsEmpty() {
		return
	}
	if r.resourceHints.EarlyHintsEnabled() {
		if err := r.sendEarlyResourceHints(c, links); err != nil && r.logger != nil {
			r.logger.Debug("Send early resource hints failed", slog.String("err", err.Error()))
		}
	}
	applyResourceHints(c, links)
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
) (string, error) {
	body, found, err := r.bodyCache.GetOrLoadWithRequest(result.FilePath, request)
	if err != nil {
		if missingErr := newMissingResolvedVariantError(result, err); missingErr != nil {
			return "", missingErr
		}
		return "", fiber.ErrInternalServerError
	}
	if err := c.Send(body); err != nil {
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
	headerPlan.ApplySendFileOverrides(c)
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

func nonNegativeSize(size int64) (int64, bool) {
	if size < 0 {
		return 0, false
	}
	return size, true
}

func variantSize(variant *catalog.Variant) (int64, bool) {
	if variant == nil {
		return 0, false
	}
	return nonNegativeSize(variant.Size)
}

func assetSize(asset *catalog.Asset) (int64, bool) {
	if asset == nil {
		return 0, false
	}
	return nonNegativeSize(asset.Size)
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
	if result == nil || result.Variant == nil || err == nil || !errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return &missingResolvedVariantError{
		artifactPath: result.Variant.ArtifactPath,
		cause:        err,
	}
}
