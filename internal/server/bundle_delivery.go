package server

import (
	"errors"
	"fmt"
	"github.com/samber/oops"
	"os"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

func readServerAssetFile(path string, files *serverFileSources) ([]byte, error) {
	if spackbundle.IsReference(path) {
		body, err := spackbundle.ReadReference(path)
		if err != nil {
			return nil, oops.Wrapf(err, "read bundle asset")
		}
		return body, nil
	}
	if files == nil {
		return nil, oops.Errorf("local file source is required for %s", path)
	}
	body, err := files.ReadFile(path)
	if err != nil {
		return nil, oops.Wrapf(err, "read local asset file")
	}
	return body, nil
}

func (r *assetDeliveryRuntime) sendResolvedBundleAsset(
	c fiber.Ctx,
	request resolver.Request,
	result *resolver.Result,
	headerPlan resolvedHeaderPlan,
) (string, error) {
	body, err := readServerAssetFile(result.FilePath, r.fileSources)
	if err != nil {
		if missingErr := newMissingResolvedVariantError(result, err); missingErr != nil {
			return "", missingErr
		}
		return "", fiber.ErrInternalServerError
	}
	return sendBundleBody(c, request, body, headerPlan)
}

func (r *assetDeliveryRuntime) sendPreparedBundleAssetFile(
	c fiber.Ctx,
	request resolver.Request,
	response *preparedResponse,
	headerPlan preparedHeaderPlan,
) (string, error) {
	body, err := readServerAssetFile(response.filePath(), r.fileSources)
	if err != nil {
		if handled, retryErr := r.retryPreparedArtifactMiss(c, request, response); handled || retryErr != nil {
			return "", retryErr
		}
		return "", oops.Wrapf(err, "send prepared bundle asset file")
	}
	return sendBundleBody(c, request, body, headerPlan)
}

func sendBundleBody(
	c fiber.Ctx,
	request resolver.Request,
	body []byte,
	headerPlan sendFileHeaderPlan,
) (string, error) {
	if request.RangeRequested {
		return sendBundleRangeBody(c, body, c.Get(fiber.HeaderRange), headerPlan)
	}
	headerPlan.ApplySendFileOverrides(c, false)
	if err := c.Send(body); err != nil {
		return "", oops.Wrapf(err, "send bundle asset body")
	}
	return deliverySendFile, nil
}

func sendBundleRangeBody(
	c fiber.Ctx,
	body []byte,
	rangeHeader string,
	headerPlan sendFileHeaderPlan,
) (string, error) {
	byteRange, ok := parseSingleHTTPRange(rangeHeader, int64(len(body)))
	if !ok {
		sendUnsatisfiedRange(c, int64(len(body)), headerPlan)
		return deliverySendFileRange, nil
	}

	c.Status(fiber.StatusPartialContent)
	c.Set(fiber.HeaderContentRange, fmt.Sprintf("bytes %d-%d/%d", byteRange.start, byteRange.end, len(body)))
	c.Set(fiber.HeaderContentLength, strconv.FormatInt(byteRange.end-byteRange.start+1, 10))
	headerPlan.ApplySendFileOverrides(c, true)
	if err := c.Send(body[byteRange.start : byteRange.end+1]); err != nil {
		return "", oops.Wrapf(err, "send bundle range body")
	}
	return deliverySendFileRange, nil
}

func sendUnsatisfiedRange(c fiber.Ctx, size int64, headerPlan sendFileHeaderPlan) {
	c.Status(fiber.StatusRequestedRangeNotSatisfiable)
	c.Set(fiber.HeaderContentRange, fmt.Sprintf("bytes */%d", size))
	c.Set(fiber.HeaderContentLength, "0")
	headerPlan.ApplySendFileOverrides(c, true)
}

type httpByteRange struct {
	start int64
	end   int64
}

func parseSingleHTTPRange(header string, size int64) (httpByteRange, bool) {
	header = strings.TrimSpace(header)
	spec, ok := strings.CutPrefix(header, "bytes=")
	if size < 0 || !ok {
		return httpByteRange{}, false
	}
	spec = strings.TrimSpace(spec)
	if spec == "" || strings.Contains(spec, ",") {
		return httpByteRange{}, false
	}
	startRaw, endRaw, ok := strings.Cut(spec, "-")
	if !ok {
		return httpByteRange{}, false
	}
	return parseHTTPRangeBounds(strings.TrimSpace(startRaw), strings.TrimSpace(endRaw), size)
}

func parseHTTPRangeBounds(startRaw, endRaw string, size int64) (httpByteRange, bool) {
	if startRaw == "" {
		return parseHTTPSuffixRange(endRaw, size)
	}
	start, err := strconv.ParseInt(startRaw, 10, 64)
	if err != nil || start < 0 || start >= size {
		return httpByteRange{}, false
	}
	end := size - 1
	if endRaw != "" {
		parsedEnd, err := strconv.ParseInt(endRaw, 10, 64)
		if err != nil || parsedEnd < start {
			return httpByteRange{}, false
		}
		end = min(parsedEnd, size-1)
	}
	return httpByteRange{start: start, end: end}, true
}

func parseHTTPSuffixRange(endRaw string, size int64) (httpByteRange, bool) {
	suffix, err := strconv.ParseInt(endRaw, 10, 64)
	if err != nil || suffix <= 0 || size == 0 {
		return httpByteRange{}, false
	}
	if suffix > size {
		suffix = size
	}
	return httpByteRange{start: size - suffix, end: size - 1}, true
}

func isMissingServerAsset(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
