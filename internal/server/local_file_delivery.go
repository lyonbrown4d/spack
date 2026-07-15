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

type serverFileGuards struct {
	guards []*source.LocalRootGuard
}

func newServerFileGuards(
	cfg *config.Config,
	src *source.LocalFS,
	cat catalog.Catalog,
	logger *slog.Logger,
) *serverFileGuards {
	return mergeServerFileGuards(
		newServerFileGuardsFromSource(src, logger),
		newServerFileGuardsFromConfig(cfg, logger),
		newServerFileGuardsFromCatalog(cat, logger),
	)
}

func newServerFileGuardsFromSource(src *source.LocalFS, logger *slog.Logger) *serverFileGuards {
	if guard, resolved := serverFileGuardFromSource(src, logger); resolved && guard != nil {
		return &serverFileGuards{guards: []*source.LocalRootGuard{guard}}
	}
	return nil
}

func newServerFileGuardsFromConfig(cfg *config.Config, logger *slog.Logger) *serverFileGuards {
	if cfg == nil {
		return nil
	}
	return mergeServerFileGuards(
		newServerFileGuardsFromRoot(cfg.Assets.Root, logger),
		newServerFileGuardsFromRoot(cfg.Compression.CacheDir, logger),
	)
}

func newServerFileGuardsFromCatalog(cat catalog.Catalog, logger *slog.Logger) *serverFileGuards {
	if cat == nil {
		return nil
	}
	return newServerFileGuardsFromRoot(catalogFileGuardRoot(cat), logger)
}

func newServerFileGuardsFromRoot(root string, logger *slog.Logger) *serverFileGuards {
	guard := serverFileGuardFromRoot(root, logger)
	if guard == nil {
		return nil
	}
	return &serverFileGuards{guards: []*source.LocalRootGuard{guard}}
}

func mergeServerFileGuards(groups ...*serverFileGuards) *serverFileGuards {
	merged := &serverFileGuards{}
	seen := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		mergeServerFileGuardGroup(merged, seen, group)
	}
	if len(merged.guards) == 0 {
		return nil
	}
	return merged
}

func mergeServerFileGuardGroup(merged *serverFileGuards, seen map[string]struct{}, group *serverFileGuards) {
	if group == nil {
		return
	}
	for _, guard := range group.guards {
		mergeServerFileGuard(merged, seen, guard)
	}
}

func mergeServerFileGuard(merged *serverFileGuards, seen map[string]struct{}, guard *source.LocalRootGuard) {
	if guard == nil || guard.Root() == "" {
		return
	}
	key := strings.ToLower(filepath.Clean(guard.Root()))
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	merged.guards = append(merged.guards, guard)
}

func newServerFileGuard(src *source.LocalFS, fallbackRoot string, logger *slog.Logger) *source.LocalRootGuard {
	if guard, resolved := serverFileGuardFromSource(src, logger); resolved {
		return guard
	}
	return serverFileGuardFromRoot(fallbackRoot, logger)
}

func serverFileGuardFromSource(src *source.LocalFS, logger *slog.Logger) (*source.LocalRootGuard, bool) {
	if src == nil {
		return nil, false
	}
	guard, ok, err := src.RootGuard()
	if err != nil {
		warnServerFileGuard(logger, "Resolved local source root guard unavailable", err)
		return nil, true
	}
	return guard, ok
}

func serverFileGuardFromRoot(fallbackRoot string, logger *slog.Logger) *source.LocalRootGuard {
	guard, ok, err := source.NewLocalRootGuard(fallbackRoot)
	if err != nil {
		warnServerFileGuard(logger, "Local source root guard unavailable", err)
		return nil
	}
	if !ok {
		return nil
	}
	return guard
}

func warnServerFileGuard(logger *slog.Logger, message string, err error) {
	if logger == nil {
		return
	}
	logger.Warn(message, slog.String("err", err.Error()))
}

func (g *serverFileGuards) ReadFile(path string) ([]byte, error) {
	file, _, err := g.OpenFile(path)
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil {
		return nil, oops.Wrapf(readErr, "read guarded asset file")
	}
	if closeErr != nil {
		return nil, oops.Wrapf(closeErr, "close guarded asset file")
	}
	return body, nil
}

func (g *serverFileGuards) OpenFile(path string) (*os.File, fs.FileInfo, error) {
	if g == nil || len(g.guards) == 0 {
		return nil, nil, oops.Owner("server").Wrap(fmt.Errorf("local file guard is required for %s", path))
	}
	errs := make([]error, 0, len(g.guards))
	for _, guard := range g.guards {
		file, info, err := guard.OpenFile(path)
		if err == nil {
			return file, info, nil
		}
		errs = append(errs, err)
	}
	return nil, nil, oops.Owner("server").Wrap(fmt.Errorf("open guarded asset file: %w", errors.Join(errs...)))
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
	if r != nil && r.fileGuards != nil {
		file, info, err := r.fileGuards.OpenFile(result.FilePath)
		if err != nil {
			return nil, nil, oops.Wrapf(err, "open guarded asset file")
		}
		return file, info, nil
	}
	return nil, nil, oops.Owner("server").Wrap(fmt.Errorf("local file guard is required for %s", result.FilePath))
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
