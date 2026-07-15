package server

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/resolver"
)

var errServerAssetDirectory = errors.New("asset path is a directory")

func (r *assetDeliveryRuntime) sendResolvedAssetFileRange(
	c fiber.Ctx,
	result *resolver.Result,
	headerPlan resolvedHeaderPlan,
) (string, error) {
	// #nosec G304 -- path comes from resolver/catalog-selected asset paths already validated against the asset tree.
	file, info, err := r.openResolvedAssetFile(result)
	if err != nil {
		if missingErr := newMissingResolvedVariantError(result, err); missingErr != nil {
			return "", missingErr
		}
		r.logSendAssetError(result, err)
		return "", fiber.ErrInternalServerError
	}

	size := info.Size()
	byteRange, ok := parseSingleHTTPRange(c.Get(fiber.HeaderRange), size)
	if !ok {
		sendUnsatisfiedRange(c, size, headerPlan)
		discardServerFile(file)
		return deliverySendFileRange, nil
	}

	return r.sendResolvedAssetFileRangeBody(c, file, size, byteRange, headerPlan)
}

func (r *assetDeliveryRuntime) sendResolvedAssetFileRangeBody(
	c fiber.Ctx,
	file *os.File,
	size int64,
	byteRange httpByteRange,
	headerPlan resolvedHeaderPlan,
) (string, error) {
	length := byteRange.end - byteRange.start + 1
	c.Status(fiber.StatusPartialContent)
	c.Set(fiber.HeaderContentRange, fmt.Sprintf("bytes %d-%d/%d", byteRange.start, byteRange.end, size))
	c.Set(fiber.HeaderContentLength, strconv.FormatInt(length, 10))
	headerPlan.ApplySendFileOverrides(c, true)
	stream := sectionReadCloser{
		SectionReader: io.NewSectionReader(file, byteRange.start, length),
		closer:        file,
	}
	if err := sendServerStream(c, stream, length, "send ranged asset body"); err != nil {
		return "", err
	}
	return deliverySendFileRange, nil
}

func (r *assetDeliveryRuntime) logSendAssetError(result *resolver.Result, err error) {
	if r.logger == nil {
		return
	}
	r.logger.Error("Send asset failed",
		slog.String("path", result.FilePath),
		slog.String("err", err.Error()),
	)
}
