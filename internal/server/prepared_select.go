package server

import (
	"path"
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/config"
	contentcodingspec "github.com/lyonbrown4d/spack/internal/contentcoding/spec"
	"github.com/lyonbrown4d/spack/internal/media"
	"github.com/lyonbrown4d/spack/internal/requestpath"
	"github.com/lyonbrown4d/spack/internal/resolver"
)

type preparedRequest struct {
	resolver.Request
	RequestedFormat string
}

type preparedSelection struct {
	response           *preparedResponse
	fallbackUsed       bool
	preferredEncodings *cxlist.List[string]
	preferredWidths    *cxlist.List[int]
	preferredFormats   *cxlist.List[string]
	explicitFormat     bool
}

func (s *preparedSnapshot) resolve(cfg config.Assets, request preparedRequest) (*preparedSelection, bool) {
	route, fallbackUsed, ok := s.findRoute(cfg, request.Path)
	if !ok || route == nil {
		return nil, false
	}
	return route.selectResponse(request, fallbackUsed), true
}

func (s *preparedSnapshot) findRoute(cfg config.Assets, requestPath string) (*preparedRoute, bool, bool) {
	cleaned := requestpath.Clean(requestPath)
	if route, ok := s.findPrimaryRoute(cfg, cleaned); ok {
		return route, false, true
	}

	if cfg.Fallback.On == config.FallbackOnNotFound && cleaned.AllowsEntryFallback {
		target := requestpath.Clean(cfg.Fallback.Target).Value
		if route, ok := s.routes.Get(target); ok {
			return route, true, true
		}
	}
	return nil, false, false
}

func (s *preparedSnapshot) findPrimaryRoute(cfg config.Assets, requestPath requestpath.Cleaned) (*preparedRoute, bool) {
	if requestPath.Value == "" {
		return s.routes.Get(cfg.Entry)
	}
	if route, ok := s.routes.Get(requestPath.Value); ok {
		return route, true
	}
	if !requestPath.AllowsEntryFallback {
		return nil, false
	}

	candidate := path.Join(requestPath.Value, cfg.Entry)
	if candidate == requestPath.Value {
		return nil, false
	}
	return s.routes.Get(candidate)
}

func (r *preparedRoute) selectResponse(request preparedRequest, fallbackUsed bool) *preparedSelection {
	selection := &preparedSelection{
		response:       r.identity,
		fallbackUsed:   fallbackUsed,
		explicitFormat: strings.TrimSpace(request.RequestedFormat) != "",
	}

	if image := r.selectImageResponse(request, selection); image != nil {
		selection.response = image
		return selection
	}
	if encoding := r.selectEncodingResponse(request, selection); encoding != nil {
		selection.response = encoding
	}
	return selection
}

func (r *preparedRoute) selectImageResponse(request preparedRequest, selection *preparedSelection) *preparedResponse {
	asset := r.identity.asset()
	if asset == nil {
		return nil
	}

	formats := resolver.PreferredImageFormats(request.Accept, request.RequestedFormat, asset.MediaType)
	selection.preferredFormats = formats
	selection.preferredWidths = resolver.PreferredWidths(request.Width)
	if request.Width <= 0 && formats.Len() == 0 {
		return nil
	}
	if formats.IsEmpty() {
		formats = cxlist.NewList[string](media.ImageFormat(asset.MediaType))
	}

	picked, _ := cxlist.FilterMapList[string, *preparedResponse](formats, func(_ int, format string) (*preparedResponse, bool) {
		response := r.pickImageFormat(format, request.Width)
		return response, response != nil
	}).GetFirst()
	return picked
}

func (r *preparedRoute) pickImageFormat(format string, width int) *preparedResponse {
	responses, ok := r.images.Get(format)
	if !ok || responses.IsEmpty() {
		return nil
	}
	if width <= 0 {
		return pickZeroWidthImageResponse(responses)
	}
	return pickClosestWidthImageResponse(responses, width)
}

func (r *preparedRoute) selectEncodingResponse(request preparedRequest, selection *preparedSelection) *preparedResponse {
	if request.RangeRequested {
		return nil
	}

	encodings := resolver.ParseAcceptEncoding(request.AcceptEncoding, contentcodingspec.DefaultNames())
	selection.preferredEncodings = encodings
	if encodings.Len() == 0 {
		return nil
	}

	picked, _ := cxlist.FilterMapList[string, *preparedResponse](encodings, func(_ int, encoding string) (*preparedResponse, bool) {
		response, ok := r.encodings.Get(encoding)
		return response, ok
	}).GetFirst()
	return picked
}

func pickZeroWidthImageResponse(responses *cxlist.List[*preparedResponse]) *preparedResponse {
	picked, _ := responses.FirstWhere(func(_ int, response *preparedResponse) bool {
		variant := response.variant()
		return variant != nil && variant.Width == 0
	}).Get()
	return picked
}

func pickClosestWidthImageResponse(responses *cxlist.List[*preparedResponse], width int) *preparedResponse {
	var smallestAbove *preparedResponse
	var largestBelow *preparedResponse
	responses.Range(func(_ int, response *preparedResponse) bool {
		variant := response.variant()
		if variant.Width >= width {
			if smallestAbove == nil || variant.Width < smallestAbove.variant().Width {
				smallestAbove = response
			}
			return true
		}
		if largestBelow == nil || variant.Width > largestBelow.variant().Width {
			largestBelow = response
		}
		return true
	})
	if smallestAbove != nil {
		return smallestAbove
	}
	return largestBelow
}
