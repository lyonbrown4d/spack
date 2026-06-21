package sourcecatalog

import (
	"cmp"
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	cxprefix "github.com/arcgolabs/collectionx/prefix"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/samber/mo"
)

type sidecarMatcher struct {
	encoding string
	suffix   string
}

type sidecarFile struct {
	source.File
	assetPath string
	encoding  string
	suffix    string
}

type sidecarVariantBuildCandidate struct {
	sidecar sidecarFile
	asset   *catalog.Asset
}

func IsSourceSidecarVariant(variant *catalog.Variant) bool {
	if variant == nil || variant.Metadata == nil {
		return false
	}
	return strings.TrimSpace(variant.Metadata.GetOrDefault("stage", "")) == SourceSidecarStage
}

func buildSidecarMatchers(registry contentcoding.Registry) *cxlist.List[sidecarMatcher] {
	matchers := cxlist.NewListWithCapacity[sidecarMatcher](registry.Names().Len())
	registry.Names().Range(func(_ int, name string) bool {
		strategy, ok := registry.Lookup(name)
		if !ok {
			return true
		}
		matchers.Add(sidecarMatcher{
			encoding: strategy.Name(),
			suffix:   strategy.Suffix(),
		})
		return true
	})
	return matchers.Sort(func(left, right sidecarMatcher) int {
		if len(left.suffix) == len(right.suffix) {
			return cmp.Compare(left.encoding, right.encoding)
		}
		return cmp.Compare(len(right.suffix), len(left.suffix))
	})
}

func buildSidecarMatcherTrie(matchers *cxlist.List[sidecarMatcher]) *cxprefix.Trie[sidecarMatcher] {
	trie := cxprefix.NewTrie[sidecarMatcher]()
	if matchers == nil || matchers.IsEmpty() {
		return trie
	}

	matchers.Range(func(_ int, matcher sidecarMatcher) bool {
		if matcher.suffix != "" {
			trie.Put(reverseString(matcher.suffix), matcher)
		}
		return true
	})
	return trie
}

func matchSidecarWithTrie(path string, matcherTrie *cxprefix.Trie[sidecarMatcher]) mo.Option[sidecarMatcher] {
	if matcherTrie == nil || matcherTrie.IsEmpty() {
		return mo.None[sidecarMatcher]()
	}
	matcherKey, matcher, ok := matcherTrie.LongestPrefix(reverseString(path))
	if !ok || matcherKey == "" {
		return mo.None[sidecarMatcher]()
	}
	return mo.Some(matcher)
}

func recognizeSidecars(filesByPath *cxmapping.Map[string, source.File], matchers *cxlist.List[sidecarMatcher]) *cxmapping.Map[string, sidecarFile] {
	matcherTrie := buildSidecarMatcherTrie(matchers)
	sidecars := cxmapping.NewMapWithCapacity[string, sidecarFile](filesByPath.Len())
	sortedKeys[source.File](filesByPath).Range(func(_ int, path string) bool {
		file := filesByPath.GetOrDefault(path, source.File{})
		if isExplicitBundleVariantFile(file) {
			return true
		}
		match, ok := matchSidecar(path, filesByPath, matcherTrie).Get()
		if !ok {
			return true
		}

		match.File = file
		sidecars.Set(match.Path, match)
		return true
	})
	return sidecars
}

func matchSidecar(path string, filesByPath *cxmapping.Map[string, source.File], matcherTrie *cxprefix.Trie[sidecarMatcher]) mo.Option[sidecarFile] {
	matcher, ok := matchSidecarWithTrie(path, matcherTrie).Get()
	if !ok {
		return mo.None[sidecarFile]()
	}

	assetPath := normalizedAssetPath(path, matcher.suffix)
	if assetPath == "" || assetPath == path {
		return mo.None[sidecarFile]()
	}
	if filesByPath.GetOption(assetPath).IsAbsent() {
		return mo.None[sidecarFile]()
	}

	return mo.Some(sidecarFile{
		assetPath: assetPath,
		encoding:  matcher.encoding,
		suffix:    matcher.suffix,
	})
}

func reverseString(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	for left, right := 0, len(runes)-1; left < right; left, right = left+1, right-1 {
		runes[left], runes[right] = runes[right], runes[left]
	}
	return string(runes)
}
