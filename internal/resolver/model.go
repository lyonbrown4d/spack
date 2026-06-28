package resolver

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"log/slog"
)

type Request struct {
	Path           string
	Accept         string
	AcceptEncoding string
	Width          int
	Format         string
	RangeRequested bool

	PreferredEncodings *cxlist.List[string]
	PreferredFormats   *cxlist.List[string]
}

type Result struct {
	Asset              *catalog.Asset
	Variant            *catalog.Variant
	FilePath           string
	MediaType          string
	ContentEncoding    string
	ETag               string
	PreferredEncodings *cxlist.List[string]
	PreferredWidths    *cxlist.List[int]
	PreferredFormats   *cxlist.List[string]
	FallbackUsed       bool
}

type Resolver struct {
	cfg                *config.Assets
	supportedEncodings encodingSupport
	catalog            catalog.Catalog
	logger             *slog.Logger
	obs                observabilityx.Observability
}
