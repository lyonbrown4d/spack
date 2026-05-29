package server

import (
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/resolver"
)

type preparedSnapshot struct {
	routes   *cxmapping.Map[string, *preparedRoute]
	builtAt  time.Time
	assets   int
	variants int
}

type preparedRoute struct {
	path        string
	identity    *preparedResponse
	encodings   *cxmapping.Map[string, *preparedResponse]
	images      *cxmapping.Map[string, *cxlist.List[*preparedResponse]]
	imageWidths *cxmapping.Table[string, int, *preparedResponse]
}

type preparedResponse struct {
	result             resolver.Result
	headerPlan         resolvedHeaderPlan
	explicitHeaderPlan resolvedHeaderPlan
	resourceHints      *cxlist.List[string]
	body               []byte
	bodyPrepared       bool
}

func newPreparedSnapshot(capacity int) *preparedSnapshot {
	return &preparedSnapshot{
		routes:  cxmapping.NewMapWithCapacity[string, *preparedRoute](capacity),
		builtAt: time.Now().UTC(),
	}
}

func newPreparedRoute(path string, identity *preparedResponse) *preparedRoute {
	return &preparedRoute{
		path:        path,
		identity:    identity,
		encodings:   cxmapping.NewMap[string, *preparedResponse](),
		images:      cxmapping.NewMap[string, *cxlist.List[*preparedResponse]](),
		imageWidths: cxmapping.NewTable[string, int, *preparedResponse](),
	}
}

func (r *preparedRoute) addVariant(response *preparedResponse) {
	if r == nil || response == nil || response.result.Variant == nil {
		return
	}
	variant := response.result.Variant
	if variant.Encoding != "" {
		r.encodings.Set(variant.Encoding, response)
		return
	}
	format := variantImageFormat(response)
	if format == "" {
		return
	}
	responses, _ := r.images.GetOrCompute(format, func() *cxlist.List[*preparedResponse] {
		return cxlist.NewList[*preparedResponse]()
	})
	r.imageWidths.Put(format, variant.Width, response)
	responses.Add(response)
}

func (r *preparedRoute) finalize() {
	if r == nil {
		return
	}
	r.images.Range(func(_ string, responses *cxlist.List[*preparedResponse]) bool {
		responses.Sort(comparePreparedImageResponses)
		return true
	})
}

func (r *preparedResponse) asset() *catalog.Asset {
	if r == nil {
		return nil
	}
	return r.result.Asset
}

func (r *preparedResponse) variant() *catalog.Variant {
	if r == nil {
		return nil
	}
	return r.result.Variant
}

func (r *preparedResponse) filePath() string {
	if r == nil {
		return ""
	}
	return r.result.FilePath
}
