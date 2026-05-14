package server

import (
	"strings"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/spack/internal/constant"
)

const RequestIDHeader = "X-Request-ID"
const poweredByHeader = "X-Powered-By"

func buildServerHeader(meta dix.AppMeta) string {
	version := strings.TrimSpace(meta.Version)
	if version == "" {
		return constant.ServerHeaderPrefix
	}
	return constant.ServerHeaderPrefix + "/" + version
}
