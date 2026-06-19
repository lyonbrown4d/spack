package server

import (
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/cachepolicy"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/samber/mo"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type resolvedHeaderPlan struct {
	contentType     string
	contentLength   mo.Option[string]
	vary            string
	etag            mo.Option[string]
	contentEncoding mo.Option[string]
	lastModified    mo.Option[string]
	cacheControl    string
	expires         mo.Option[string]
}

func newResolvedHeaderPlan(
	policy cachepolicy.ResponsePolicy,
	result *resolver.Result,
	requestedFormat string,
) resolvedHeaderPlan {
	lastModified := resolvedLastModified(result)
	cacheControl := policy.CacheControl(result)
	size, hasSize := resolvedAssetSize(result)
	contentLength := mo.TupleToOption[string](strconv.FormatInt(size, 10), hasSize)
	expiresAt, hasExpires := policy.ExpiresAt(cacheControl, lastModified.modTime.OrEmpty(), lastModified.modTime.IsPresent())
	expires := mo.TupleToOption[string](expiresAt.UTC().Format(http.TimeFormat), hasExpires)

	sourceMediaType := ""
	if result != nil && result.Asset != nil {
		sourceMediaType = result.Asset.MediaType
	}

	return resolvedHeaderPlan{
		contentType:     result.MediaType,
		contentLength:   contentLength,
		vary:            resolvedVaryHeader(sourceMediaType, requestedFormat),
		etag:            mo.EmptyableToOption(strings.TrimSpace(result.ETag)),
		contentEncoding: mo.EmptyableToOption(strings.TrimSpace(result.ContentEncoding)),
		lastModified:    lastModified.header,
		cacheControl:    cacheControl,
		expires:         expires,
	}
}

func (p resolvedHeaderPlan) Apply(c fiber.Ctx) {
	p.ApplyForRange(c, false)
}

func (p resolvedHeaderPlan) ApplyForRange(c fiber.Ctx, rangeRequested bool) {
	c.Set(fiber.HeaderContentType, p.contentType)
	c.Set(fiber.HeaderVary, p.vary)
	c.Set(fiber.HeaderCacheControl, p.cacheControl)
	if !rangeRequested {
		applyOptionalHeader(c, fiber.HeaderContentLength, p.contentLength)
	}
	applyOptionalHeader(c, fiber.HeaderETag, p.etag)
	applyOptionalHeader(c, fiber.HeaderContentEncoding, p.contentEncoding)
	applyOptionalHeader(c, fiber.HeaderLastModified, p.lastModified)
	applyOptionalHeader(c, fiber.HeaderExpires, p.expires)
}

func (p resolvedHeaderPlan) ApplySendFileOverrides(c fiber.Ctx, rangeRequested bool) {
	c.Set(fiber.HeaderContentType, p.contentType)
	if !rangeRequested {
		applyOptionalHeader(c, fiber.HeaderContentLength, p.contentLength)
	}
	applyOptionalHeader(c, fiber.HeaderContentEncoding, p.contentEncoding)
	applyOptionalHeader(c, fiber.HeaderLastModified, p.lastModified)
}

func applyOptionalHeader(c fiber.Ctx, key string, value mo.Option[string]) {
	if headerValue, ok := value.Get(); ok {
		c.Set(key, headerValue)
		return
	}
	c.Response().Header.Del(key)
}

func shouldSendNotModified(c fiber.Ctx, request resolver.Request) bool {
	if request.RangeRequested {
		return false
	}
	return c.Req().Fresh()
}

type resolvedLastModifiedValue struct {
	modTime mo.Option[time.Time]
	header  mo.Option[string]
}

func resolvedLastModified(result *resolver.Result) resolvedLastModifiedValue {
	if result == nil {
		return resolvedLastModifiedValue{
			modTime: mo.None[time.Time](),
			header:  mo.None[string](),
		}
	}

	if value, ok := metadataLastModifiedValue(result.Variant.GetMetadata()); ok {
		return value
	}
	if value, ok := metadataLastModifiedValue(result.Asset.GetMetadata()); ok {
		return value
	}
	if modTime, ok := catalog.FileModTime(result.FilePath).Get(); ok {
		return resolvedLastModifiedValue{
			modTime: mo.Some(modTime),
			header:  mo.Some(modTime.UTC().Format(http.TimeFormat)),
		}
	}
	return resolvedLastModifiedValue{
		modTime: mo.None[time.Time](),
		header:  mo.None[string](),
	}
}

func metadataLastModifiedValue(metadata *cxmapping.Map[string, string]) (resolvedLastModifiedValue, bool) {
	modTime, hasModTime := catalog.MetadataModTime(metadata).Get()
	header, hasHeader := catalog.MetadataLastModifiedHTTP(metadata).Get()
	if !hasModTime && !hasHeader {
		return resolvedLastModifiedValue{}, false
	}

	value := resolvedLastModifiedValue{
		modTime: mo.None[time.Time](),
		header:  mo.None[string](),
	}
	if hasModTime {
		value.modTime = mo.Some(modTime)
	}
	if hasHeader {
		value.header = mo.Some(header)
	}
	return value, true
}

func resolvedVaryHeader(sourceMediaType, requestedFormat string) string {
	if shouldVaryAccept(sourceMediaType, requestedFormat) {
		return fiber.HeaderAcceptEncoding + ", " + fiber.HeaderAccept
	}
	return fiber.HeaderAcceptEncoding
}
