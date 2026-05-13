package server

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/arcgolabs/eventx"
	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/cachepolicy"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/media"
	"github.com/lyonbrown4d/spack/internal/pipeline"
	"github.com/lyonbrown4d/spack/internal/requestpath"
	"github.com/lyonbrown4d/spack/internal/resolver"
)

const maxVariantFallbackAttempts = 3

type assetDeliveryRuntime struct {
	mountPath      string
	responsePolicy cachepolicy.ResponsePolicy
	logger         *slog.Logger
	assetResolver  *resolver.Resolver
	pipelineSvc    *pipeline.Service
	bodyCache      *assetcache.Cache
	bus            eventx.BusRuntime
	prepared       *PreparedService
	trackDelivery  bool
	resourceHints  *resourceHintService
}

type assetDeliveryRuntimeDeps struct {
	cfg           *config.Config
	routeRuntime  assetRouteRuntime
	assetResolver *resolver.Resolver
	pipelineSvc   *pipeline.Service
	bodyCache     *assetcache.Cache
	bus           eventx.BusRuntime
}

func registerAssetRoute(app *fiber.App, runtime *assetDeliveryRuntime) {
	if runtime == nil {
		return
	}
	app.Use(routePattern(runtime.mountPath), runtime.handle)
}

func newAssetDeliveryRuntime(deps assetDeliveryRuntimeDeps) *assetDeliveryRuntime {
	return &assetDeliveryRuntime{
		mountPath:      deps.cfg.Assets.Path,
		responsePolicy: cachepolicy.NewResponsePolicyFromConfig(deps.cfg),
		logger:         deps.routeRuntime.logger,
		assetResolver:  deps.assetResolver,
		pipelineSvc:    deps.pipelineSvc,
		bodyCache:      deps.bodyCache,
		bus:            deps.bus,
		prepared:       deps.routeRuntime.prepared,
		trackDelivery:  deps.routeRuntime.trackDelivery,
		resourceHints:  deps.routeRuntime.resourceHints,
	}
}

func (r *assetDeliveryRuntime) handle(c fiber.Ctx) error {
	requestedFormat := media.NormalizeImageFormat(c.Query("format"))
	request := buildResolverRequest(c, r.mountPath, requestedFormat)
	if r.prepared != nil {
		if selection, ok := r.prepared.Resolve(preparedRequest{Request: request, RequestedFormat: requestedFormat}); ok {
			delivery, result, err := r.sendPreparedAsset(c, request, selection)
			if err != nil {
				return err
			}
			r.afterAssetServed(c, delivery, result)
			return nil
		}
		return fiber.ErrNotFound
	}

	result, err := r.assetResolver.Resolve(c.Context(), request)
	if err != nil {
		return fiber.ErrNotFound
	}

	r.enqueuePipelineResult(result)
	delivery, resolvedResult, deliveryErr := r.sendResolvedAssetWithVariantFallback(c, request, result, requestedFormat)
	if deliveryErr != nil {
		return deliveryErr
	}
	r.afterAssetServed(c, delivery, resolvedResult)
	return nil
}

func (r *assetDeliveryRuntime) afterAssetServed(c fiber.Ctx, delivery string, result *resolver.Result) {
	if delivery != "" {
		if r.trackDelivery {
			setAssetDelivery(c, delivery)
		}
		publishVariantServed(c.Context(), result, r.bus, r.logger)
	}
}

func (r *assetDeliveryRuntime) sendResolvedAssetWithVariantFallback(
	c fiber.Ctx,
	request resolver.Request,
	result *resolver.Result,
	requestedFormat string,
) (string, *resolver.Result, error) {
	delivery, err := r.sendResolvedAsset(c, request, result, requestedFormat)
	if err == nil {
		return delivery, result, nil
	}

	missingErr := asMissingResolvedVariantError(err)
	if missingErr == nil {
		return "", result, err
	}

	return r.retryResolvedAssetDelivery(c, request, result, requestedFormat, missingErr)
}

func (r *assetDeliveryRuntime) retryResolvedAssetDelivery(
	c fiber.Ctx,
	request resolver.Request,
	result *resolver.Result,
	requestedFormat string,
	missingErr *missingResolvedVariantError,
) (string, *resolver.Result, error) {
	for range maxVariantFallbackAttempts {
		nextResult, resolveErr := r.resolveAfterVariantArtifactMiss(c.Context(), request, result, missingErr)
		if resolveErr != nil {
			return "", result, resolveErr
		}
		r.enqueuePipelineResult(nextResult)

		delivery, err := r.sendResolvedAsset(c, request, nextResult, requestedFormat)
		if err == nil {
			return delivery, nextResult, nil
		}

		missingErr = asMissingResolvedVariantError(err)
		if missingErr == nil {
			return "", nextResult, err
		}
		result = nextResult
	}

	return "", result, fiber.ErrInternalServerError
}

func (r *assetDeliveryRuntime) resolveAfterVariantArtifactMiss(
	ctx context.Context,
	request resolver.Request,
	result *resolver.Result,
	missingErr *missingResolvedVariantError,
) (*resolver.Result, error) {
	if r.bodyCache != nil {
		r.bodyCache.Delete(missingErr.artifactPath)
	}

	nextResult, err := r.assetResolver.ResolveAfterVariantArtifactMiss(ctx, request, result.Variant)
	if err != nil {
		return nil, fiber.ErrNotFound
	}
	return nextResult, nil
}

func asMissingResolvedVariantError(err error) *missingResolvedVariantError {
	if missingErr, ok := errors.AsType[*missingResolvedVariantError](err); ok {
		return missingErr
	}
	return nil
}

func buildResolverRequest(c fiber.Ctx, mountPath, requestedFormat string) resolver.Request {
	cleanedPath := requestpath.CleanMounted(c.Path(), mountPath)
	acceptHeader := strings.TrimSpace(c.Get(fiber.HeaderAccept))
	acceptEncodingHeader := strings.TrimSpace(c.Get(fiber.HeaderAcceptEncoding))

	return resolver.Request{
		Path:           cleanedPath.Value,
		Accept:         acceptHeader,
		AcceptEncoding: acceptEncodingHeader,
		Width:          parsePositiveInt(c.Query("w")),
		Format:         requestedFormat,
		RangeRequested: requestRangeRequested(c),
	}
}

func requestRangeRequested(c fiber.Ctx) bool {
	return strings.TrimSpace(c.Get(fiber.HeaderRange)) != ""
}

func (r *assetDeliveryRuntime) enqueuePipelineResult(result *resolver.Result) {
	if r.pipelineSvc == nil || result == nil || result.Asset == nil || !hasPreferredPipelineRequests(result) {
		return
	}

	r.pipelineSvc.Enqueue(pipeline.Request{
		AssetPath:          result.Asset.Path,
		PreferredEncodings: result.PreferredEncodings,
		PreferredWidths:    result.PreferredWidths,
		PreferredFormats:   result.PreferredFormats,
	})
}

func hasPreferredPipelineRequests(result *resolver.Result) bool {
	if result == nil {
		return false
	}
	return result.PreferredEncodings.Len() > 0 ||
		result.PreferredWidths.Len() > 0 ||
		result.PreferredFormats.Len() > 0
}

func parsePositiveInt(raw string) int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}
