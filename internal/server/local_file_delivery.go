package server

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
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
		return oops.Wrapf(err, "close section stream")
	}
	return nil
}

type serverFileSources struct {
	sources []*source.LocalFS
}

func newServerFileSources(
	cfg *config.Config,
	src *source.LocalFS,
	cat catalog.Catalog,
	logger *slog.Logger,
) *serverFileSources {
	return mergeServerFileSources(
		newServerFileSourcesFromSource(src),
		newServerFileSourcesFromConfig(cfg, logger),
		newServerFileSourcesFromCatalog(cat, logger),
	)
}

func newServerFileSourcesFromSource(src *source.LocalFS) *serverFileSources {
	if src != nil && src.Root() != "" {
		return &serverFileSources{sources: []*source.LocalFS{src}}
	}
	return nil
}

func newServerFileSourcesFromConfig(cfg *config.Config, logger *slog.Logger) *serverFileSources {
	if cfg == nil {
		return nil
	}
	return mergeServerFileSources(
		newServerFileSourcesFromRoot(cfg.Assets.Root, logger),
		newServerFileSourcesFromOptionalRoot(cfg.Compression.CacheDir, logger),
	)
}

func newServerFileSourcesFromCatalog(cat catalog.Catalog, logger *slog.Logger) *serverFileSources {
	if cat == nil {
		return nil
	}
	return newServerFileSourcesFromRoot(catalogFileSourceRoot(cat), logger)
}

func newServerFileSourcesFromRoot(root string, logger *slog.Logger) *serverFileSources {
	files := serverFileSourceFromRoot(root, logger)
	if files == nil {
		return nil
	}
	return &serverFileSources{sources: []*source.LocalFS{files}}
}

func newServerFileSourcesFromOptionalRoot(root string, logger *slog.Logger) *serverFileSources {
	files := serverFileSourceFromOptionalRoot(root, logger)
	if files == nil {
		return nil
	}
	return &serverFileSources{sources: []*source.LocalFS{files}}
}

func mergeServerFileSources(groups ...*serverFileSources) *serverFileSources {
	merged := &serverFileSources{}
	seen := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		mergeServerFileSourceGroup(merged, seen, group)
	}
	if len(merged.sources) == 0 {
		return nil
	}
	return merged
}

func mergeServerFileSourceGroup(merged *serverFileSources, seen map[string]struct{}, group *serverFileSources) {
	if group == nil {
		return
	}
	for _, files := range group.sources {
		mergeServerFileSource(merged, seen, files)
	}
}

func mergeServerFileSource(merged *serverFileSources, seen map[string]struct{}, files *source.LocalFS) {
	if files == nil || files.Root() == "" {
		return
	}
	key := strings.ToLower(filepath.Clean(files.Root()))
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	merged.sources = append(merged.sources, files)
}

func newServerFileSource(src *source.LocalFS, fallbackRoot string, logger *slog.Logger) *source.LocalFS {
	if src != nil && src.Root() != "" {
		return src
	}
	return serverFileSourceFromRoot(fallbackRoot, logger)
}

func serverFileSourceFromRoot(fallbackRoot string, logger *slog.Logger) *source.LocalFS {
	files, ok, err := source.NewLocalDirectory(fallbackRoot)
	if err != nil {
		warnServerFileSource(logger, "Local file source unavailable", err)
		return nil
	}
	if !ok {
		return nil
	}
	return files
}

func serverFileSourceFromOptionalRoot(fallbackRoot string, logger *slog.Logger) *source.LocalFS {
	files, ok, err := source.NewLocalDirectory(fallbackRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		warnServerFileSource(logger, "Local file source unavailable", err)
		return nil
	}
	if !ok {
		return nil
	}
	return files
}

func warnServerFileSource(logger *slog.Logger, message string, err error) {
	if logger == nil {
		return
	}
	logger.Warn(message, slog.Any("error", err))
}

func (s *serverFileSources) ReadFile(path string) ([]byte, error) {
	file, _, err := s.OpenFile(path)
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil {
		return nil, oops.Wrapf(readErr, "read local asset file")
	}
	if closeErr != nil {
		return nil, oops.Wrapf(closeErr, "close local asset file")
	}
	return body, nil
}

func (s *serverFileSources) OpenFile(path string) (*os.File, fs.FileInfo, error) {
	if s == nil || len(s.sources) == 0 {
		return nil, nil, oops.Owner("server").Wrap(fmt.Errorf("local file source is required for %s", path))
	}
	errs := make([]error, 0, len(s.sources))
	for _, files := range s.sources {
		file, info, err := files.OpenFile(path)
		if err == nil {
			return file, info, nil
		}
		errs = append(errs, err)
	}
	return nil, nil, oops.Owner("server").Wrap(fmt.Errorf("open local asset file: %w", errors.Join(errs...)))
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
