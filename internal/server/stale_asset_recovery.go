package server

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/cachepolicy"
	"github.com/samber/oops"
)

const staleRecoveryAssetPlaceholder = "__SPACK_STALE_ASSET__"

//go:embed stale_recovery.js
var staleRecoveryScript []byte

func (r *assetDeliveryRuntime) serveStaleAssetRecovery(c fiber.Ctx, assetPath string) error {
	encodedAssetPath, err := json.Marshal(assetPath)
	if err != nil {
		return oops.Wrapf(err, "encode stale asset recovery path")
	}
	body := bytes.Replace(
		staleRecoveryScript,
		[]byte(staleRecoveryAssetPlaceholder),
		encodedAssetPath,
		1,
	)

	c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJavaScript)
	c.Set(fiber.HeaderCacheControl, "no-store")
	if err := c.Send(body); err != nil {
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
	return r.serveStaleAssetRecovery(c, assetPath)
}
