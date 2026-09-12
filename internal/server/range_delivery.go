package server

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/resolver"
)

type singleByteRangeDisposition uint8

const (
	singleByteRangeSatisfiable singleByteRangeDisposition = iota
	singleByteRangeUnsupported
	singleByteRangeRejected
)

func resolveSingleByteRange(c fiber.Ctx, size int64) (fiber.RangeSet, singleByteRangeDisposition) {
	parsedRange, err := c.Range(size)
	if errors.Is(err, fiber.ErrRangeUnsupported) {
		return fiber.RangeSet{}, singleByteRangeUnsupported
	}
	if err != nil || strings.Contains(c.Get(fiber.HeaderRange), ",") || len(parsedRange.Ranges) != 1 {
		return fiber.RangeSet{}, singleByteRangeRejected
	}
	return parsedRange.Ranges[0], singleByteRangeSatisfiable
}

func (r *assetDeliveryRuntime) sendResolvedAssetFileRange(
	c fiber.Ctx,
	result *resolver.Result,
	headerPlan resolvedHeaderPlan,
) (string, error) {
	file, info, err := r.openResolvedAssetFile(result)
	if err != nil {
		if missingErr := newMissingResolvedVariantError(result, err); missingErr != nil {
			return "", missingErr
		}
		r.logSendAssetError(result, err)
		return "", fiber.ErrInternalServerError
	}

	size := info.Size()
	byteRange, disposition := resolveSingleByteRange(c, size)
	if disposition == singleByteRangeUnsupported {
		headerPlan.ApplySendFileOverrides(c, false)
		if err := sendServerStream(c, file, size, "send guarded asset body"); err != nil {
			return "", err
		}
		return deliverySendFile, nil
	}
	if disposition != singleByteRangeSatisfiable {
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
	byteRange fiber.RangeSet,
	headerPlan resolvedHeaderPlan,
) (string, error) {
	length := byteRange.End - byteRange.Start + 1
	c.Status(fiber.StatusPartialContent)
	c.Set(fiber.HeaderContentRange, fmt.Sprintf("bytes %d-%d/%d", byteRange.Start, byteRange.End, size))
	c.Set(fiber.HeaderContentLength, strconv.FormatInt(length, 10))
	headerPlan.ApplySendFileOverrides(c, true)
	stream := sectionReadCloser{
		SectionReader: io.NewSectionReader(file, byteRange.Start, length),
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
		slog.Any("error", err),
	)
}
