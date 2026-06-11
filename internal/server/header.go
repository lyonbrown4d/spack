package server

import (
	"strings"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/constant"
)

const RequestIDHeader = "X-Request-ID"
const poweredByHeader = "X-Powered-By"

func buildServerHeader(cfg *config.Config, meta dix.AppMeta) string {
	exposeVersion := cfg != nil && cfg.HTTP.ExposeServerVersion
	version := strings.TrimSpace(meta.Version)
	if !exposeVersion || version == "" {
		return constant.ServerHeaderPrefix
	}
	return constant.ServerHeaderPrefix + "/" + version
}
