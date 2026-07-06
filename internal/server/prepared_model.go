package server

import (
	"sort"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/samber/lo"
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

type preparedImagePickCandidate struct {
	width    int
	response *preparedResponse
}

type preparedResponse struct {
	result             resolver.Result
	headerPlan         preparedHeaderPlan
	explicitHeaderPlan preparedHeaderPlan
	resourceHints      *cxlist.List[string]
	resourceHintHeader string
	body               []byte
	bodyPrepared       bool
	servedResult       *resolver.Result
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

	candidates := lo.FilterMap(responses.Values(), func(response *preparedResponse, _ int) (preparedImagePickCandidate, bool) {
		variant := response.variant()
		if variant == nil || variant.Width < 0 {
			return preparedImagePickCandidate{}, false
		}
		return preparedImagePickCandidate{width: variant.Width, response: response}, true
	})
	candidates = lo.UniqBy(candidates, func(candidate preparedImagePickCandidate) int {
		return candidate.width
	})
	if len(candidates) == 0 {
		return nil
	}
	return &preparedImagePicker{
		widths: lo.Map(candidates, func(candidate preparedImagePickCandidate, _ int) int {
			return candidate.width
		}),
		responses: lo.Map(candidates, func(candidate preparedImagePickCandidate, _ int) *preparedResponse {
			return candidate.response
		}),
	}
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
