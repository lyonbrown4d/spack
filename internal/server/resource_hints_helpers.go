package server

import (
	"errors"
	"github.com/samber/oops"
	"io"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	cxset "github.com/arcgolabs/collectionx/set"
	"golang.org/x/net/html"

	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/media"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/samber/lo"
	"github.com/samber/mo"
)

func parseHTMLResourceHints(filePath string, cfg config.ResourceHints, guard *source.LocalRootGuard) (links *cxlist.List[string], err error) {
	cleanPath := filepath.Clean(filePath)
	if guard == nil {
		return nil, oops.Errorf("local source root guard is required for %s", cleanPath)
	}
	file, _, err := guard.OpenFile(cleanPath)
	if err != nil {
		return nil, oops.Wrapf(err, "open guarded HTML asset")
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			err = errors.Join(err, oops.Wrapf(cerr, "close HTML asset"))
		}
	}()

	links, err = collectResourceHintsFromHTML(io.LimitReader(file, maxResourceHintScanBytes), cfg)
	return links, err
}

func tokenizerHTMLParseError(tokenizer *html.Tokenizer) error {
	err := tokenizer.Err()
	if err == nil || errors.Is(err, io.EOF) {
		return nil
	}
	return oops.Wrapf(err, "parse HTML asset")
}

func consumeStartTagResourceHint(
	tokenizer *html.Tokenizer,
	links *cxlist.List[string],
	seen *cxset.OrderedSet[string],
	cfg config.ResourceHints,
	headerBytes *int,
) bool {
	name, _ := tokenizer.TagName()
	hint, ok := resourceHintFromTag(string(name), htmlTagAttrs(tokenizer))
	if !ok {
		return true
	}
	return appendResourceHint(links, seen, hint, cfg, headerBytes)
}

func collectResourceHintsFromHTML(r io.Reader, cfg config.ResourceHints) (*cxlist.List[string], error) {
	tokenizer := html.NewTokenizer(r)
	links := cxlist.NewList[string]()
	seen := cxset.NewOrderedSet[string]()
	headerBytes := 0

	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			if err := tokenizerHTMLParseError(tokenizer); err != nil {
				return links, err
			}
			return links, nil
		case html.StartTagToken, html.SelfClosingTagToken:
			if !consumeStartTagResourceHint(tokenizer, links, seen, cfg, &headerBytes) {
				return links, nil
			}
		case html.TextToken, html.EndTagToken, html.CommentToken, html.DoctypeToken:
		}
	}
}

func appendResourceHint(
	links *cxlist.List[string],
	seen *cxset.OrderedSet[string],
	hint resourceHint,
	cfg config.ResourceHints,
	headerBytes *int,
) bool {
	header, ok := hint.Header()
	if !ok || seen.Contains(header) {
		return true
	}
	if links.Len() >= cfg.LinkLimit() {
		return false
	}

	nextBytes := *headerBytes + len(header)
	if links.Len() > 0 {
		nextBytes += len(", ")
	}
	if nextBytes > cfg.HeaderByteLimit() {
		return false
	}

	seen.Add(header)
	links.Add(header)
	*headerBytes = nextBytes
	return true
}

func resourceHintFromTag(tag string, attrs *cxmapping.Map[string, string]) (resourceHint, bool) {
	switch strings.ToLower(tag) {
	case "script":
		return scriptResourceHint(attrs)
	case "link":
		return linkResourceHint(attrs)
	default:
		return resourceHint{}, false
	}
}

func scriptResourceHint(attrs *cxmapping.Map[string, string]) (resourceHint, bool) {
	src := attrs.GetOrDefault("src", "")
	if !isValidResourceHintURL(src) {
		return resourceHint{}, false
	}

	scriptType := strings.ToLower(strings.TrimSpace(attrs.GetOrDefault("type", "")))
	if scriptType == "module" {
		return resourceHint{url: src, rel: "modulepreload", crossorigin: attrs.GetOrDefault("crossorigin", "")}, true
	}
	if scriptType == "" || strings.Contains(scriptType, "javascript") {
		return resourceHint{url: src, rel: "preload", as: "script", crossorigin: attrs.GetOrDefault("crossorigin", "")}, true
	}
	return resourceHint{}, false
}

func linkResourceHint(attrs *cxmapping.Map[string, string]) (resourceHint, bool) {
	href := attrs.GetOrDefault("href", "")
	if !isValidResourceHintURL(href) {
		return resourceHint{}, false
	}

	relValues := splitRelValues(attrs.GetOrDefault("rel", ""))
	switch {
	case relValues.Contains("stylesheet"):
		return resourceHint{url: href, rel: "preload", as: "style", crossorigin: attrs.GetOrDefault("crossorigin", "")}, true
	case relValues.Contains("modulepreload"):
		return resourceHint{url: href, rel: "modulepreload", crossorigin: attrs.GetOrDefault("crossorigin", "")}, true
	case relValues.Contains("preload"):
		return preloadResourceHint(href, attrs)
	case relValues.Contains("prefetch"):
		return resourceHint{url: href, rel: "prefetch", as: attrs.GetOrDefault("as", ""), crossorigin: attrs.GetOrDefault("crossorigin", "")}, true
	case relValues.Contains("preconnect"):
		return resourceHint{url: href, rel: "preconnect", crossorigin: attrs.GetOrDefault("crossorigin", "")}, true
	case relValues.Contains("dns-prefetch"):
		return resourceHint{url: href, rel: "dns-prefetch"}, true
	default:
		return resourceHint{}, false
	}
}

func preloadResourceHint(href string, attrs *cxmapping.Map[string, string]) (resourceHint, bool) {
	as := strings.ToLower(strings.TrimSpace(attrs.GetOrDefault("as", "")))
	if as == "" {
		as = inferResourceHintAs(href)
	}
	if as == "" {
		return resourceHint{}, false
	}
	crossorigin := attrs.GetOrDefault("crossorigin", "")
	if as == "font" && crossorigin == "" {
		crossorigin = "anonymous"
	}
	return resourceHint{url: href, rel: "preload", as: as, crossorigin: crossorigin}, true
}

func (h resourceHint) Header() (string, bool) {
	if !isValidResourceHintURL(h.url) || h.rel == "" {
		return "", false
	}

	parts := lo.Compact([]string{
		"<" + h.url + ">",
		"rel=" + h.rel,
		lo.Ternary(h.as != "", "as="+h.as, ""),
		resourceHintCrossoriginHeader(h.crossorigin),
	})
	return strings.Join(parts, "; "), true
}

func resourceHintCrossoriginHeader(crossorigin string) string {
	switch strings.ToLower(strings.TrimSpace(crossorigin)) {
	case "":
		return ""
	case "anonymous":
		return "crossorigin"
	default:
		return "crossorigin=" + crossorigin
	}
}

func htmlTagAttrs(tokenizer *html.Tokenizer) *cxmapping.Map[string, string] {
	attrs := cxmapping.NewMap[string, string]()
	for {
		key, value, more := tokenizer.TagAttr()
		attrs.Set(strings.ToLower(string(key)), string(value))
		if !more {
			return attrs
		}
	}
}

func splitRelValues(raw string) *cxset.OrderedSet[string] {
	return cxset.NewOrderedSet[string](strings.Fields(strings.ToLower(raw))...)
}

func inferResourceHintAs(rawURL string) string {
	switch strings.ToLower(path.Ext(resourceHintURLPath(rawURL))) {
	case ".js", ".mjs":
		return "script"
	case ".css":
		return "style"
	case ".woff", ".woff2", ".ttf", ".otf":
		return "font"
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".avif", ".svg":
		return "image"
	default:
		return ""
	}
}

func resourceHintURLPath(rawURL string) string {
	value := rawURL
	if before, _, found := strings.Cut(value, "?"); found {
		value = before
	}
	if before, _, found := strings.Cut(value, "#"); found {
		value = before
	}
	return value
}

func isResourceHintHTML(mediaType string) bool {
	normalized := media.NormalizeMediaType(mediaType)
	return strings.HasPrefix(normalized, "text/html") || strings.Contains(normalized, "application/xhtml")
}

func isValidResourceHintURL(raw string) bool {
	value := strings.TrimSpace(raw)
	if value == "" || strings.HasPrefix(value, "#") {
		return false
	}
	if strings.ContainsAny(value, "\r\n<>") || strings.ContainsAny(value, " \t") {
		return false
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}

	switch strings.ToLower(parsed.Scheme) {
	case "", "http", "https":
		return true
	default:
		return false
	}
}

func resourceHintCacheKey(asset *catalog.Asset) string {
	if asset == nil {
		return ""
	}
	suffix := mo.EmptyableToOption(strings.TrimSpace(asset.SourceHash)).OrElse(strings.TrimSpace(asset.ETag))
	if suffix == "" {
		return asset.FullPath
	}
	return asset.FullPath + "|" + suffix
}
