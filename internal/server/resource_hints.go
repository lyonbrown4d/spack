package server

import (
	"github.com/samber/oops"
	"log/slog"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/source"
)

const maxResourceHintScanBytes = 512 * 1024

type resourceHintService struct {
	cfg       config.ResourceHints
	logger    *slog.Logger
	cache     *cxmapping.ConcurrentMap[string, resourceHintCacheEntry]
	fileGuard *source.LocalRootGuard
}

type resourceHintCacheEntry struct {
	links  *cxlist.List[string]
	header string
}

type resourceHintLinks = *cxlist.List[string]

type resourceHint struct {
	url         string
	rel         string
	as          string
	crossorigin string
}

func newResourceHintService(cfg *config.Config, logger *slog.Logger, src *source.LocalFS) *resourceHintService {
	var hints config.ResourceHints
	var fallbackRoot string
	if cfg != nil {
		hints = cfg.Frontend.ResourceHints
		fallbackRoot = cfg.Assets.Root
	}
	return &resourceHintService{
		cfg:       hints,
		logger:    logger,
		cache:     cxmapping.NewConcurrentMap[string, resourceHintCacheEntry](),
		fileGuard: newServerFileGuard(src, fallbackRoot, logger),
	}
}

func (s *resourceHintService) Links(result *resolver.Result) *cxlist.List[string] {
	entry, ok := s.Entry(result)
	if !ok {
		return nil
	}
	return entry.links
}

func (s *resourceHintService) Entry(result *resolver.Result) (resourceHintCacheEntry, bool) {
	if s == nil || !s.cfg.Enabled() || result == nil || result.Asset == nil {
		return resourceHintCacheEntry{}, false
	}
	if !isResourceHintHTML(result.Asset.MediaType) {
		return resourceHintCacheEntry{}, false
	}

	key := resourceHintCacheKey(result.Asset)
	if cached, ok := s.cache.Get(key); ok {
		return cached, true
	}

	links, err := parseHTMLResourceHints(result.Asset.FullPath, s.cfg, s.fileGuard)
	if err != nil && s.logger != nil {
		s.logger.Debug("Parse HTML resource hints failed",
			slog.String("path", result.Asset.FullPath),
			slog.String("err", err.Error()),
		)
	}
	entry := resourceHintCacheEntry{
		links:  links,
		header: resourceHintHeader(links),
	}
	s.cache.Set(key, entry)
	return entry, true
}

func (s *resourceHintService) EarlyHintsEnabled() bool {
	return s != nil && s.cfg.Enabled() && s.cfg.EarlyHints
}

func resourceHintHeader(links *cxlist.List[string]) string {
	if links == nil || links.IsEmpty() {
		return ""
	}
	return links.Join(", ")
}

func (r *assetDeliveryRuntime) sendEarlyResourceHints(c fiber.Ctx, links *cxlist.List[string]) error {
	if links == nil || links.IsEmpty() {
		return nil
	}
	var err error
	links.ViewValues(func(values []string) {
		err = c.SendEarlyHints(values)
	})
	if err != nil {
		return oops.Wrapf(err, "send early resource hints")
	}
	return nil
}
