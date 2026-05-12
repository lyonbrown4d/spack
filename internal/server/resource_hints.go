package server

import (
	"fmt"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	cxset "github.com/arcgolabs/collectionx/set"
	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"log/slog"
)

const maxResourceHintScanBytes = 512 * 1024

type resourceHintService struct {
	cfg        config.ResourceHints
	logger     *slog.Logger
	cache      *cxmapping.ConcurrentMultiMap[string, string]
	cachedKeys *cxset.ConcurrentSet[string]
}

type resourceHint struct {
	url         string
	rel         string
	as          string
	crossorigin string
}

func newResourceHintService(cfg *config.Frontend, logger *slog.Logger) *resourceHintService {
	var hints config.ResourceHints
	if cfg != nil {
		hints = cfg.ResourceHints
	}
	return &resourceHintService{
		cfg:        hints,
		logger:     logger,
		cache:      cxmapping.NewConcurrentMultiMap[string, string](),
		cachedKeys: cxset.NewConcurrentSet[string](),
	}
}

func (s *resourceHintService) Links(result *resolver.Result) *cxlist.List[string] {
	if s == nil || !s.cfg.Enabled() || result == nil || result.Asset == nil {
		return nil
	}
	if !isResourceHintHTML(result.Asset.MediaType) {
		return nil
	}

	key := resourceHintCacheKey(result.Asset)
	if cached, ok := s.cached(key); ok {
		return cached
	}

	links, err := parseHTMLResourceHints(result.Asset.FullPath, s.cfg)
	if err != nil && s.logger != nil {
		s.logger.Debug("Parse HTML resource hints failed",
			slog.String("path", result.Asset.FullPath),
			slog.String("err", err.Error()),
		)
	}
	s.store(key, links)
	return links
}

func (s *resourceHintService) EarlyHintsEnabled() bool {
	return s != nil && s.cfg.Enabled() && s.cfg.EarlyHints
}

func (s *resourceHintService) cached(key string) (*cxlist.List[string], bool) {
	if !s.cachedKeys.Contains(key) {
		return nil, false
	}
	return cxlist.NewList[string](s.cache.GetCopy(key)...), true
}

func (s *resourceHintService) store(key string, links *cxlist.List[string]) {
	s.cache.Set(key, links.Values()...)
	s.cachedKeys.Add(key)
}

func applyResourceHints(c fiber.Ctx, links *cxlist.List[string]) {
	if links == nil || links.IsEmpty() {
		return
	}
	c.Set(fiber.HeaderLink, links.Join(", "))
}

func (r *assetDeliveryRuntime) sendEarlyResourceHints(c fiber.Ctx, links *cxlist.List[string]) error {
	if links == nil || links.IsEmpty() {
		return nil
	}
	if err := c.SendEarlyHints(links.Values()); err != nil {
		return fmt.Errorf("send early resource hints: %w", err)
	}
	return nil
}
