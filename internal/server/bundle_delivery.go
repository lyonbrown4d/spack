package server

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/oops"
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
		return sendBundleRangeBody(c, body, headerPlan)
	}
	return sendBundleFullBody(c, body, headerPlan)
}

func sendBundleFullBody(
	c fiber.Ctx,
	body []byte,
	headerPlan sendFileHeaderPlan,
) (string, error) {
	headerPlan.ApplySendFileOverrides(c, false)
	if err := c.Send(body); err != nil {
		return "", oops.Wrapf(err, "send bundle asset body")
	}
	return deliverySendFile, nil
}

func sendBundleRangeBody(
	c fiber.Ctx,
	body []byte,
	headerPlan sendFileHeaderPlan,
) (string, error) {
	byteRange, disposition := resolveSingleByteRange(c, int64(len(body)))
	if disposition == singleByteRangeUnsupported {
		return sendBundleFullBody(c, body, headerPlan)
	}
	if disposition != singleByteRangeSatisfiable {
		sendUnsatisfiedRange(c, int64(len(body)), headerPlan)
		return deliverySendFileRange, nil
	}

	c.Status(fiber.StatusPartialContent)
	c.Set(fiber.HeaderContentRange, fmt.Sprintf("bytes %d-%d/%d", byteRange.Start, byteRange.End, len(body)))
	c.Set(fiber.HeaderContentLength, strconv.FormatInt(byteRange.End-byteRange.Start+1, 10))
	headerPlan.ApplySendFileOverrides(c, true)
	if err := c.Send(body[byteRange.Start : byteRange.End+1]); err != nil {
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

func isMissingServerAsset(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
