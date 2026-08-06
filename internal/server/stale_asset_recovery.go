package server

import (
	_ "embed"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/cachepolicy"
	"github.com/samber/oops"
)

//go:embed stale_recovery.js
var staleRecoveryScript []byte

func (r *assetDeliveryRuntime) serveStaleAssetRecovery(c fiber.Ctx) error {
	c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJavaScript)
	c.Set(fiber.HeaderCacheControl, "no-store")
	if err := c.Send(staleRecoveryScript); err != nil {
		return oops.Wrapf(err, "send stale asset recovery script")
	}
	return nil
}

func (r *assetDeliveryRuntime) tryStaleAssetRecovery(c fiber.Ctx, assetPath string) error {
	if !r.staleAssetRecovery.Enable {
		return fiber.ErrNotFound
	}
	if !strings.HasSuffix(assetPath, ".js") {
		return fiber.ErrNotFound
	}
	if !cachepolicy.IsFingerprintAssetPath(assetPath) {
		return fiber.ErrNotFound
	}
	return r.serveStaleAssetRecovery(c)
}
