package server

import (
	"fmt"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/lo"
)

func (r *assetDeliveryRuntime) sendPreparedAsset(
	c fiber.Ctx,
	request resolver.Request,
	selection preparedSelection,
) (string, *resolver.Result, error) {
	response := selection.response
	headerPlan := lo.Ternary(selection.explicitFormat, response.explicitHeaderPlan, response.headerPlan)

	headerPlan.ApplyForRange(c, request.RangeRequested)
	if handled := handleConditionalAssetRequest(c, request); handled {
		return "", nil, nil
	}
	r.applyPreparedResourceHints(c, request, response)

	if response.bodyPrepared && !request.RangeRequested {
		if err := c.Send(response.body); err != nil {
			return "", nil, fmt.Errorf("send prepared asset body: %w", err)
		}
		return deliveryPreparedMemory, preparedServedVariantResult(selection), nil
	}

	delivery, err := r.sendPreparedAssetFile(c, request, selection, headerPlan)
	if err != nil {
		return "", nil, err
	}
	return delivery, preparedServedVariantResult(selection), nil
}

func preparedServedVariantResult(selection preparedSelection) *resolver.Result {
	if selection.response == nil || selection.response.variant() == nil {
		return nil
	}
	return &selection.response.result
}

func (r *assetDeliveryRuntime) sendPreparedAssetFile(
	c fiber.Ctx,
	request resolver.Request,
	selection preparedSelection,
	headerPlan resolvedHeaderPlan,
) (string, error) {
	response := selection.response
	if spackbundle.IsReference(response.filePath()) {
		return r.sendPreparedBundleAssetFile(c, request, response, headerPlan)
	}
	if err := c.SendFile(response.filePath(), fiber.SendFile{ByteRange: true}); err != nil {
		if handled, retryErr := r.retryPreparedArtifactMiss(c, request, response); handled || retryErr != nil {
			return "", retryErr
		}
		return "", fmt.Errorf("send prepared asset file: %w", err)
	}
	headerPlan.ApplySendFileOverrides(c, request.RangeRequested)
	if request.RangeRequested {
		return deliverySendFileRange, nil
	}
	return deliveryPreparedFile, nil
}

func (r *assetDeliveryRuntime) retryPreparedArtifactMiss(
	c fiber.Ctx,
	request resolver.Request,
	response *preparedResponse,
) (bool, error) {
	if r.prepared == nil || response == nil || response.variant() == nil {
		return false, nil
	}
	if !r.prepared.DeleteVariantArtifact(c.Context(), response.filePath()) {
		return false, nil
	}
	next, ok := r.prepared.Resolve(newPreparedRequest(request, request.Format)).Get()
	if !ok || next.response == nil || next.response.filePath() == response.filePath() {
		return false, nil
	}
	_, _, err := r.sendPreparedAsset(c, request, next)
	return true, err
}

func (r *assetDeliveryRuntime) applyPreparedResourceHints(
	c fiber.Ctx,
	request resolver.Request,
	response *preparedResponse,
) {
	if response == nil || c.Method() != fiber.MethodGet || request.RangeRequested {
		return
	}
	links := response.resourceHints
	if links == nil || links.IsEmpty() {
		return
	}
	if r.resourceHints != nil && r.resourceHints.EarlyHintsEnabled() {
		if err := r.sendEarlyResourceHints(c, links); err != nil && r.logger != nil {
			r.logger.Debug("Send early resource hints failed", "err", err.Error())
		}
	}
	applyResourceHints(c, links)
}
