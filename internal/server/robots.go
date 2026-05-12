package server

import (
	"embed"
	"log/slog"
	"strings"
	"text/template"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/cachepolicy"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/samber/oops"
)

const robotsAssetPath = "robots.txt"
const robotAssetsRoute = "/robots.txt"

//go:embed templates/robots.txt.tmpl
var robotsTemplateFS embed.FS

var robotsContentTemplate = template.Must(template.ParseFS(robotsTemplateFS, "templates/robots.txt.tmpl"))

type robotsTemplateData struct {
	UserAgent string
	Allow     string
	Disallow  string
	Host      string
	Sitemap   string
}

func registerRobotsRoute(
	app *fiber.App,
	cfg *config.Config,
	logger *slog.Logger,
	cat catalog.Catalog,
	bodyCache *assetcache.Cache,
) {
	if !cfg.Robots.Enable {
		return
	}

	deliveryRuntime := &assetDeliveryRuntime{
		responsePolicy: cachepolicy.NewResponsePolicyFromConfig(cfg),
		logger:         logger,
		bodyCache:      bodyCache,
	}
	handler := func(c fiber.Ctx) error {
		if asset, ok := staticRobotsAsset(cfg.Robots, cat); ok {
			_, err := deliveryRuntime.sendResolvedAsset(
				c,
				resolver.Request{RangeRequested: requestRangeRequested(c)},
				&resolver.Result{
					Asset:     asset,
					FilePath:  asset.FullPath,
					MediaType: asset.MediaType,
					ETag:      asset.ETag,
				},
				"",
			)
			return err
		}
		return sendGeneratedRobots(c, cfg.Robots)
	}

	app.Get(robotAssetsRoute, handler)
	app.Head(robotAssetsRoute, handler)
}

func staticRobotsAsset(cfg config.Robots, cat catalog.Catalog) (*catalog.Asset, bool) {
	if cfg.Override {
		return nil, false
	}
	return cat.FindAsset(robotsAssetPath)
}

func sendGeneratedRobots(c fiber.Ctx, cfg config.Robots) error {
	body, err := renderRobotsContent(cfg)
	if err != nil {
		return err
	}

	c.Set(fiber.HeaderContentType, "text/plain; charset=utf-8")
	c.Status(fiber.StatusOK)
	if c.Method() == fiber.MethodHead {
		return nil
	}
	if err := c.SendString(body); err != nil {
		return oops.In("server").Owner("robots").Wrap(err)
	}
	return nil
}

func renderRobotsContent(cfg config.Robots) (string, error) {
	allow := strings.TrimSpace(cfg.Allow)
	disallow := strings.TrimSpace(cfg.Disallow)
	if allow == "" && disallow == "" {
		allow = "/"
	}

	var body strings.Builder
	err := robotsContentTemplate.Execute(&body, robotsTemplateData{
		UserAgent: cfg.NormalizedUserAgent(),
		Allow:     allow,
		Disallow:  disallow,
		Host:      strings.TrimSpace(cfg.Host),
		Sitemap:   strings.TrimSpace(cfg.Sitemap),
	})
	if err != nil {
		return "", oops.In("server").Owner("robots").Wrap(err)
	}

	return strings.TrimRight(body.String(), "\n") + "\n", nil
}
