package server

import (
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/resolver"
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
		return oops.Wrapf(err, "close section stream")
	}
	return nil
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

func (r *assetDeliveryRuntime) openResolvedAssetFile(result *resolver.Result) (*os.File, fs.FileInfo, error) {
	if r != nil && r.fileSources != nil {
		file, info, err := r.fileSources.OpenFile(result.FilePath)
		if err != nil {
			return nil, nil, oops.Wrapf(err, "open local asset file")
		}
		return file, info, nil
	}
	return nil, nil, oops.Owner("server").Wrap(fmt.Errorf("local file source is required for %s", result.FilePath))
}

func sendServerStream(c fiber.Ctx, stream io.Reader, size int64, action string) error {
	if size < 0 || size > int64(math.MaxInt) {
		discardServerStream(stream)
		return oops.Errorf("%s size is outside supported range: %d", action, size)
	}
	if err := c.SendStream(stream, int(size)); err != nil {
		discardServerStream(stream)
		return oops.Wrapf(err, "send server stream: %s", action)
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
