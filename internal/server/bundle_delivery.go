package server

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

func readServerAssetFile(path string) ([]byte, error) {
	if spackbundle.IsReference(path) {
		body, err := spackbundle.ReadReference(path)
		if err != nil {
			return nil, fmt.Errorf("read bundle asset: %w", err)
		}
		return body, nil
	}
	// #nosec G304 -- path comes from resolver/catalog-selected asset paths already validated against the asset tree.
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read asset file: %w", err)
	}
	return body, nil
}

func (r *assetDeliveryRuntime) sendResolvedBundleAsset(
	c fiber.Ctx,
	request resolver.Request,
	result *resolver.Result,
	headerPlan resolvedHeaderPlan,
) (string, error) {
	body, err := readServerAssetFile(result.FilePath)
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
	headerPlan resolvedHeaderPlan,
) (string, error) {
	body, err := readServerAssetFile(response.filePath())
	if err != nil {
		if handled, retryErr := r.retryPreparedArtifactMiss(c, request, response); handled || retryErr != nil {
			return "", retryErr
		}
		return "", fmt.Errorf("send prepared bundle asset file: %w", err)
	}
	return sendBundleBody(c, request, body, headerPlan)
}

func sendBundleBody(
	c fiber.Ctx,
	request resolver.Request,
	body []byte,
	headerPlan resolvedHeaderPlan,
) (string, error) {
	if request.RangeRequested {
		return sendBundleRangeBody(c, body, c.Get(fiber.HeaderRange), headerPlan)
	}
	headerPlan.ApplySendFileOverrides(c, false)
	if err := c.Send(body); err != nil {
		return "", fmt.Errorf("send bundle asset body: %w", err)
	}
	return deliverySendFile, nil
}

func sendBundleRangeBody(
	c fiber.Ctx,
	body []byte,
	rangeHeader string,
	headerPlan resolvedHeaderPlan,
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
		return "", fmt.Errorf("send bundle range body: %w", err)
	}
	return deliverySendFileRange, nil
}

func sendUnsatisfiedRange(c fiber.Ctx, size int64, headerPlan resolvedHeaderPlan) {
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
	if size < 0 || !strings.HasPrefix(header, "bytes=") {
		return httpByteRange{}, false
	}
	spec := strings.TrimSpace(strings.TrimPrefix(header, "bytes="))
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
