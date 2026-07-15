package server

import (
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/samber/oops"
)

type sectionReadCloser struct {
	*io.SectionReader
	closer io.Closer
}

func (r sectionReadCloser) Close() error {
	if r.closer == nil {
		return nil
	}
	if err := r.closer.Close(); err != nil {
		return fmt.Errorf("close section stream: %w", err)
	}
	return nil
}

func newServerFileGuard(root string, logger *slog.Logger) *source.LocalRootGuard {
	guard, ok, err := source.NewLocalRootGuard(root)
	if err != nil {
		if logger != nil {
			logger.Warn("Local source root guard unavailable", slog.String("err", err.Error()))
		}
		return nil
	}
	if !ok {
		return nil
	}
	return guard
}

func (r *assetDeliveryRuntime) sendResolvedAssetFileStream(
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
	headerPlan.ApplySendFileOverrides(c, false)
	if err := sendServerStream(c, file, info.Size(), "send guarded asset body"); err != nil {
		return "", err
	}
	return deliverySendFile, nil
}

func (r *assetDeliveryRuntime) openResolvedAssetFile(result *resolver.Result) (*os.File, os.FileInfo, error) {
	if r != nil && r.fileGuard != nil {
		file, info, err := r.fileGuard.OpenFile(result.FilePath)
		if err != nil {
			return nil, nil, oops.Wrapf(err, "open guarded asset file")
		}
		return file, info, nil
	}
	file, err := os.Open(result.FilePath)
	if err != nil {
		return nil, nil, fmt.Errorf("open asset file: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		discardServerFile(file)
		return nil, nil, fmt.Errorf("stat asset file: %w", err)
	}
	if info.IsDir() {
		discardServerFile(file)
		return nil, nil, errServerAssetDirectory
	}
	return file, info, nil
}

func sendServerStream(c fiber.Ctx, stream io.Reader, size int64, action string) error {
	if size < 0 || size > int64(math.MaxInt) {
		discardServerStream(stream)
		return fmt.Errorf("%s size is outside supported range: %d", action, size)
	}
	if err := c.SendStream(stream, int(size)); err != nil {
		discardServerStream(stream)
		return fmt.Errorf("%s: %w", action, err)
	}
	return nil
}

func discardServerStream(stream io.Reader) {
	closer, ok := stream.(io.Closer)
	if !ok {
		return
	}
	if err := closer.Close(); err != nil {
		return
	}
}

func discardServerFile(file *os.File) {
	if file == nil {
		return
	}
	if err := file.Close(); err != nil {
		return
	}
}
