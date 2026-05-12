package server

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/spack/internal/constant"
)

const RequestIDHeader = "X-Request-ID"

func buildServerHeader(meta dix.AppMeta) string {
	return cxlist.NewList[string](constant.ServerHeaderPrefix, meta.Version).Join("/")
}
