package server

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

var preparedManagedResponseHeaders = [...]string{
	fiber.HeaderContentType,
	fiber.HeaderContentLength,
	fiber.HeaderVary,
	fiber.HeaderETag,
	fiber.HeaderContentEncoding,
	fiber.HeaderLastModified,
	fiber.HeaderCacheControl,
	fiber.HeaderExpires,
}

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
			return "", nil, oops.Wrapf(err, "send prepared asset body")
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
	if selection.response == nil {
		return nil
	}
	return selection.response.servedResult
}

func (r *assetDeliveryRuntime) sendPreparedAssetFile(
	c fiber.Ctx,
	request resolver.Request,
	selection preparedSelection,
	headerPlan preparedHeaderPlan,
) (string, error) {
	response := selection.response
	if spackbundle.IsReference(response.filePath()) {
		return r.sendPreparedBundleAssetFile(c, request, response, headerPlan)
	}
	return r.sendPreparedLocalAssetFile(c, request, response, headerPlan)
}

func (r *assetDeliveryRuntime) sendPreparedLocalAssetFile(
	c fiber.Ctx,
	request resolver.Request,
	response *preparedResponse,
	headerPlan preparedHeaderPlan,
) (string, error) {
	if !request.RangeRequested && response.sendFile != nil {
		return r.sendPreparedTrustedAssetStream(c, request, response)
	}
	if r.fileSources == nil {
		return "", oops.Errorf("local file source is required for %s", response.filePath())
	}
	file, info, err := r.fileSources.OpenFile(response.filePath())
	if err != nil {
		if handled, retryErr := r.retryPreparedArtifactMiss(c, request, response); handled || retryErr != nil {
			return "", retryErr
		}
		return "", oops.Wrapf(err, "open prepared asset file")
	}
	if request.RangeRequested {
		return sendPreparedLocalRange(c, file, info.Size(), headerPlan)
	}
	headerPlan.ApplySendFileOverrides(c, false)
	if err := sendServerStream(c, file, info.Size(), "send prepared asset file"); err != nil {
		return "", err
	}
	return deliveryPreparedFile, nil
}

func (r *assetDeliveryRuntime) sendPreparedTrustedAssetStream(
	c fiber.Ctx,
	request resolver.Request,
	response *preparedResponse,
) (string, error) {
	file, err := response.sendFile.root.Open(response.sendFile.path)
	if err != nil {
		return r.handlePreparedTrustedFileError(c, request, response, err, "open")
	}

	info, err := file.Stat()
	if err != nil {
		discardServerStream(file)
		return r.handlePreparedTrustedFileError(c, request, response, err, "stat")
	}
	if err := sendServerStream(c, file, info.Size(), "send prepared trusted asset body"); err != nil {
		return "", err
	}
	return deliveryPreparedFile, nil
}

func (r *assetDeliveryRuntime) handlePreparedTrustedFileError(
	c fiber.Ctx,
	request resolver.Request,
	response *preparedResponse,
	cause error,
	action string,
) (string, error) {
	if errors.Is(cause, fs.ErrNotExist) {
		if handled, retryErr := r.retryPreparedArtifactMiss(c, request, response); handled || retryErr != nil {
			return "", retryErr
		}
	}
	return "", oops.Wrapf(cause, "%s prepared trusted asset file", action)
}
func sendPreparedLocalRange(c fiber.Ctx, file io.ReaderAt, size int64, headerPlan preparedHeaderPlan) (string, error) {
	byteRange, disposition := resolveSingleByteRange(c, size)
	closer, hasCloser := file.(io.Closer)
	if disposition == singleByteRangeUnsupported {
		headerPlan.ApplySendFileOverrides(c, false)
		stream := sectionReadCloser{
			SectionReader: io.NewSectionReader(file, 0, size),
			closer:        closer,
		}
		if err := sendServerStream(c, stream, size, "send prepared asset file"); err != nil {
			return "", err
		}
		return deliveryPreparedFile, nil
	}
	if disposition != singleByteRangeSatisfiable {
		sendUnsatisfiedRange(c, size, headerPlan)
		if hasCloser {
			closePreparedReader(closer)
		}
		return deliverySendFileRange, nil
	}
	length := byteRange.End - byteRange.Start + 1
	c.Status(fiber.StatusPartialContent)
	c.Set(fiber.HeaderContentRange, fmt.Sprintf("bytes %d-%d/%d", byteRange.Start, byteRange.End, size))
	c.Set(fiber.HeaderContentLength, strconv.FormatInt(length, 10))
	headerPlan.ApplySendFileOverrides(c, true)
	stream := sectionReadCloser{
		SectionReader: io.NewSectionReader(file, byteRange.Start, length),
		closer:        closer,
	}
	if err := sendServerStream(c, stream, length, "send prepared ranged asset body"); err != nil {
		return "", err
	}
	return deliverySendFileRange, nil
}

func closePreparedReader(closer io.Closer) {
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil {
		return
	}
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
	for _, header := range preparedManagedResponseHeaders {
		c.Response().Header.Del(header)
	}
	_, _, err := r.sendPreparedAsset(c, request, next)
	return true, err
}

func (r *assetDeliveryRuntime) applyPreparedResourceHints(
	c fiber.Ctx,
	request resolver.Request,
	response *preparedResponse,
) {
	if !canApplyPreparedResourceHints(c, request, response) {
		return
	}
	links := response.resourceHints
	if links == nil || links.IsEmpty() {
		return
	}
	r.sendPreparedEarlyResourceHints(c, links)
	if response.resourceHintHeader != "" {
		c.Set(fiber.HeaderLink, response.resourceHintHeader)
	}
}

func canApplyPreparedResourceHints(c fiber.Ctx, request resolver.Request, response *preparedResponse) bool {
	return response != nil && c.Method() == fiber.MethodGet && !request.RangeRequested
}

func (r *assetDeliveryRuntime) sendPreparedEarlyResourceHints(c fiber.Ctx, links resourceHintLinks) {
	if r.resourceHints == nil || !r.resourceHints.EarlyHintsEnabled() {
		return
	}
	if err := r.sendEarlyResourceHints(c, links); err != nil && r.logger != nil {
		r.logger.Debug("Send early resource hints failed", slog.Any("error", err))
	}
}
