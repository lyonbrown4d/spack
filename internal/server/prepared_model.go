package server

import (
	"sort"
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
	imagePicks  *cxmapping.Map[string, *preparedImagePicker]
	imageWidths *cxmapping.Table[string, int, *preparedResponse]
}

type preparedImagePicker struct {
	widths    []int
	responses []*preparedResponse
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
		imagePicks:  cxmapping.NewMap[string, *preparedImagePicker](),
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
	r.images.Range(func(format string, responses *cxlist.List[*preparedResponse]) bool {
		responses.Sort(comparePreparedImageResponses)
		if picker := newPreparedImagePicker(responses); picker != nil {
			r.imagePicks.Set(format, picker)
		}
		return true
	})
}

func newPreparedImagePicker(responses *cxlist.List[*preparedResponse]) *preparedImagePicker {
	if responses == nil || responses.IsEmpty() {
		return nil
	}

	picker := &preparedImagePicker{
		widths:    make([]int, 0, responses.Len()),
		responses: make([]*preparedResponse, 0, responses.Len()),
	}
	responses.Range(func(_ int, response *preparedResponse) bool {
		variant := response.variant()
		if variant == nil || variant.Width < 0 {
			return true
		}
		if len(picker.widths) > 0 && picker.widths[len(picker.widths)-1] == variant.Width {
			return true
		}
		picker.widths = append(picker.widths, variant.Width)
		picker.responses = append(picker.responses, response)
		return true
	})
	if len(picker.widths) == 0 {
		return nil
	}
	return picker
}

func (p *preparedImagePicker) closest(width int) *preparedResponse {
	if p == nil || width <= 0 || len(p.widths) == 0 {
		return nil
	}
	index := sort.SearchInts(p.widths, width)
	if index < len(p.responses) {
		return p.responses[index]
	}
	return p.responses[len(p.responses)-1]
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
