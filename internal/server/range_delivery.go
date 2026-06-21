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
	file, err := os.Open(result.FilePath)
	if err != nil {
		if missingErr := newMissingResolvedVariantError(result, err); missingErr != nil {
			return "", missingErr
		}
		r.logSendAssetError(result, err)
		return "", fiber.ErrInternalServerError
	}
	defer closeServerFile(r.logger, file, result.FilePath)

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		if err == nil {
			err = errServerAssetDirectory
		}
		r.logSendAssetError(result, err)
		return "", fiber.ErrInternalServerError
	}

	size := info.Size()
	byteRange, ok := parseSingleHTTPRange(c.Get(fiber.HeaderRange), size)
	if !ok {
		sendUnsatisfiedRange(c, size, headerPlan)
		return deliverySendFileRange, nil
	}

	return r.sendResolvedAssetFileRangeBody(c, file, result, size, byteRange, headerPlan)
}

func (r *assetDeliveryRuntime) sendResolvedAssetFileRangeBody(
	c fiber.Ctx,
	file *os.File,
	result *resolver.Result,
	size int64,
	byteRange httpByteRange,
	headerPlan resolvedHeaderPlan,
) (string, error) {
	length := byteRange.end - byteRange.start + 1
	body, err := io.ReadAll(io.NewSectionReader(file, byteRange.start, length))
	if err != nil {
		r.logSendAssetError(result, err)
		return "", fiber.ErrInternalServerError
	}

	c.Status(fiber.StatusPartialContent)
	c.Set(fiber.HeaderContentRange, fmt.Sprintf("bytes %d-%d/%d", byteRange.start, byteRange.end, size))
	c.Set(fiber.HeaderContentLength, strconv.FormatInt(length, 10))
	headerPlan.ApplySendFileOverrides(c, true)
	if err := c.Send(body); err != nil {
		return "", fmt.Errorf("send ranged asset body: %w", err)
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

func closeServerFile(logger *slog.Logger, file *os.File, path string) {
	if err := file.Close(); err != nil && logger != nil {
		logger.Debug("Close asset file failed",
			slog.String("path", path),
			slog.String("err", err.Error()),
		)
	}
}
