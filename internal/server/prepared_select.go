package server

import (
	"path"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/config"
	contentcodingspec "github.com/lyonbrown4d/spack/internal/contentcoding/spec"
	"github.com/lyonbrown4d/spack/internal/media"
	"github.com/lyonbrown4d/spack/internal/requestpath"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/samber/mo"
)

var preparedDefaultEncodings = contentcodingspec.DefaultNames()

type preparedRequest struct {
	resolver.Request
	RequestedFormat string
	CleanedPath     requestpath.Cleaned
}

func newPreparedRequest(request resolver.Request, requestedFormat string) preparedRequest {
	return preparedRequest{
		Request:         request,
		RequestedFormat: requestedFormat,
		CleanedPath:     requestpath.Clean(request.Path),
	}
}

type preparedSelection struct {
	response       *preparedResponse
	fallbackUsed   bool
	explicitFormat bool
}

type preparedRouteMatch struct {
	route        *preparedRoute
	fallbackUsed bool
}

func (s *preparedSnapshot) resolve(cfg config.Assets, request preparedRequest) mo.Option[preparedSelection] {
	match, ok := s.findRoute(cfg, request.CleanedPath).Get()
	if !ok || match.route == nil {
		return mo.None[preparedSelection]()
	}
	return mo.Some(match.route.selectResponse(request, match.fallbackUsed))
}

func (s *preparedSnapshot) findRoute(cfg config.Assets, requestPath requestpath.Cleaned) mo.Option[preparedRouteMatch] {
	if route, ok := s.findPrimaryRoute(cfg, requestPath).Get(); ok {
		return mo.Some(preparedRouteMatch{route: route})
	}

	if cfg.Fallback.On == config.FallbackOnNotFound && requestPath.AllowsEntryFallback {
		target := requestpath.Clean(cfg.Fallback.Target).Value
		if route, ok := s.routes.GetOption(target).Get(); ok {
			return mo.Some(preparedRouteMatch{route: route, fallbackUsed: true})
		}
	}
	return mo.None[preparedRouteMatch]()
}

func (s *preparedSnapshot) findPrimaryRoute(cfg config.Assets, requestPath requestpath.Cleaned) mo.Option[*preparedRoute] {
	if requestPath.Value == "" {
		return s.routes.GetOption(cfg.Entry)
	}
	if route, ok := s.routes.GetOption(requestPath.Value).Get(); ok {
		return mo.Some(route)
	}
	if !requestPath.AllowsEntryFallback {
		return mo.None[*preparedRoute]()
	}

	candidate := path.Join(requestPath.Value, cfg.Entry)
	if candidate == requestPath.Value {
		return mo.None[*preparedRoute]()
	}
	return s.routes.GetOption(candidate)
}

func (r *preparedRoute) selectResponse(request preparedRequest, fallbackUsed bool) preparedSelection {
	selection := preparedSelection{
		response:       r.identity,
		fallbackUsed:   fallbackUsed,
		explicitFormat: request.RequestedFormat != "",
	}

	if image := r.selectImageResponse(request); image != nil {
		selection.response = image
		return selection
	}
	if encoding := r.selectEncodingResponse(request); encoding != nil {
		selection.response = encoding
	}
	return selection
}

func (r *preparedRoute) selectImageResponse(request preparedRequest) *preparedResponse {
	if r.images.IsEmpty() {
		return nil
	}

	asset := r.identity.asset()
	if asset == nil {
		return nil
	}

	formats := resolver.PreferredImageFormats(request.Accept, request.RequestedFormat, asset.MediaType)
	if request.Width <= 0 && formats.Len() == 0 {
		return nil
	}
	if formats.IsEmpty() {
		formats = cxlist.NewList[string](media.ImageFormat(asset.MediaType))
	}

	var picked *preparedResponse
	formats.Range(func(_ int, format string) bool {
		picked = r.pickImageFormat(format, request.Width)
		return picked == nil
	})
	return picked
}

func (r *preparedRoute) pickImageFormat(format string, width int) *preparedResponse {
	responses, ok := r.images.GetOption(format).Get()
	if !ok || responses.IsEmpty() {
		return nil
	}
	if width <= 0 {
		if response, ok := r.imageWidths.GetOption(format, 0).Get(); ok {
			return response
		}
		return pickZeroWidthImageResponse(responses)
	}
	if response, ok := r.imageWidths.GetOption(format, width).Get(); ok {
		return response
	}
	return pickClosestWidthImageResponse(responses, width)
}

func (r *preparedRoute) selectEncodingResponse(request preparedRequest) *preparedResponse {
	if request.RangeRequested || r.encodings.IsEmpty() {
		return nil
	}

	encodings := resolver.ParseAcceptEncodingNormalized(request.AcceptEncoding, preparedDefaultEncodings)
	if encodings.Len() == 0 {
		return nil
	}

	var picked *preparedResponse
	encodings.Range(func(_ int, encoding string) bool {
		if response, ok := r.encodings.GetOption(encoding).Get(); ok {
			picked = response
		}
		return picked == nil
	})
	return picked
}

func pickZeroWidthImageResponse(responses *cxlist.List[*preparedResponse]) *preparedResponse {
	var picked *preparedResponse
	responses.Range(func(_ int, response *preparedResponse) bool {
		variant := response.variant()
		if variant == nil || variant.Width != 0 {
			return true
		}
		picked = response
		return false
	})
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
