package sourcecatalog

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	cxprefix "github.com/arcgolabs/collectionx/prefix"
	"github.com/daiyuang/spack/internal/catalog"
	"github.com/daiyuang/spack/internal/contentcoding"
	"github.com/daiyuang/spack/internal/source"
	"github.com/daiyuang/spack/pkg"
	"github.com/samber/mo"
	"github.com/samber/oops"
	"golang.org/x/sync/errgroup"
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
	return cxlist.FlatMapList[string, sidecarMatcher](registry.Names(), func(_ int, name string) []sidecarMatcher {
		strategy, ok := registry.Lookup(name)
		if !ok {
			return nil
		}
		return []sidecarMatcher{{
			encoding: strategy.Name(),
			suffix:   strategy.Suffix(),
		}}
	}).Sort(func(left, right sidecarMatcher) int {
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
	if !ok || len(matcherKey) == 0 {
		return mo.None[sidecarMatcher]()
	}
	return mo.Some(matcher)
}

func recognizeSidecars(filesByPath *cxmapping.Map[string, source.File], matchers *cxlist.List[sidecarMatcher]) *cxmapping.Map[string, sidecarFile] {
	matcherTrie := buildSidecarMatcherTrie(matchers)
	sidecars := cxmapping.NewMapWithCapacity[string, sidecarFile](filesByPath.Len())
	sortedKeys[source.File](filesByPath).Range(func(_ int, path string) bool {
		match, ok := matchSidecar(path, filesByPath, matcherTrie).Get()
		if !ok {
			return true
		}

		match.File = filesByPath.GetOrDefault(path, source.File{})
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
	if _, exists := filesByPath.Get(assetPath); !exists {
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

func buildSidecarVariants(
	ctx context.Context,
	sidecars *cxmapping.Map[string, sidecarFile],
	assets *cxmapping.Map[string, *catalog.Asset],
	existingSidecars *cxmapping.Map[string, *catalog.Variant],
) (*cxmapping.Map[string, *catalog.Variant], error) {
	variants, candidates := collectSidecarVariantBuildCandidates(sidecars, assets, existingSidecars)
	if candidates.IsEmpty() {
		return variants, nil
	}

	pending, err := buildSidecarVariantsForCandidates(ctx, candidates)
	if err != nil {
		return nil, oops.In("sourcecatalog").Owner("sidecar build").Wrap(err)
	}

	publishSidecarVariants(pending.Snapshot(), variants)
	return variants, nil
}

func buildSidecarVariantsForCandidates(
	ctx context.Context,
	candidates *cxlist.List[sidecarVariantBuildCandidate],
) (*cxlist.ConcurrentList[*catalog.Variant], error) {
	results := cxlist.NewConcurrentListWithCapacity[*catalog.Variant](candidates.Len())
	for range candidates.Len() {
		results.Add(nil)
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(sourceScanBuildParallelism(candidates.Len()))
	candidates.Range(func(index int, candidate sidecarVariantBuildCandidate) bool {
		group.Go(func() error {
			if err := scanContextErr(groupCtx); err != nil {
				return err
			}
			variant, err := buildSidecarVariant(candidate.sidecar, candidate.asset)
			if err != nil {
				return err
			}
			results.Set(index, variant)
			return nil
		})
		return true
	})
	if err := group.Wait(); err != nil {
		return nil, oops.In("sourcecatalog").Owner("sidecar build").Wrap(err)
	}
	return results, nil
}

func publishSidecarVariants(
	results *cxlist.List[*catalog.Variant],
	variants *cxmapping.Map[string, *catalog.Variant],
) {
	results.Range(func(_ int, variant *catalog.Variant) bool {
		if variant == nil {
			return true
		}
		variants.Set(variant.ID, variant)
		return true
	})
}

func collectSidecarVariantBuildCandidates(
	sidecars *cxmapping.Map[string, sidecarFile],
	assets *cxmapping.Map[string, *catalog.Asset],
	existingSidecars *cxmapping.Map[string, *catalog.Variant],
) (*cxmapping.Map[string, *catalog.Variant], *cxlist.List[sidecarVariantBuildCandidate]) {
	variants := cxmapping.NewMapWithCapacity[string, *catalog.Variant](sidecars.Len())
	candidates := cxlist.NewList[sidecarVariantBuildCandidate]()

	sortedKeys[sidecarFile](sidecars).Range(func(_ int, sidecarPath string) bool {
		sidecar, _ := sidecars.Get(sidecarPath)
		asset, ok := assets.Get(sidecar.assetPath)
		if !ok || asset == nil {
			return true
		}
		if variant, ok := reusableSidecarVariant(existingSidecars, sidecar, asset).Get(); ok {
			variants.Set(variant.ID, variant)
			return true
		}
		candidates.Add(sidecarVariantBuildCandidate{sidecar: sidecar, asset: asset})
		return true
	})
	return variants, candidates
}

func reusableSidecarVariant(
	existingSidecars *cxmapping.Map[string, *catalog.Variant],
	sidecar sidecarFile,
	asset *catalog.Asset,
) mo.Option[*catalog.Variant] {
	variant, ok := existingSidecars.Get(sidecar.FullPath)
	if !ok || !canReuseSidecarVariant(variant, sidecar, asset) {
		return mo.None[*catalog.Variant]()
	}
	updateReusableSidecarVariant(variant, sidecar, asset)
	return mo.Some(variant)
}

func updateReusableSidecarVariant(variant *catalog.Variant, sidecar sidecarFile, asset *catalog.Asset) {
	variant.ID = asset.Path + sidecar.suffix
	variant.AssetPath = asset.Path
	variant.ArtifactPath = sidecar.FullPath
	variant.Size = sidecar.Size
	variant.Encoding = sidecar.encoding
	variant.SourceHash = asset.SourceHash
	variant.MediaType = asset.MediaType
	variant.Metadata = catalog.MetadataWithModTime(variant.Metadata, sidecar.ModTime)
}

func canReuseSidecarVariant(variant *catalog.Variant, sidecar sidecarFile, asset *catalog.Asset) bool {
	if variant == nil || asset == nil || !IsSourceSidecarVariant(variant) {
		return false
	}
	modTime, ok := catalog.MetadataModTime(variant.Metadata).Get()
	return ok &&
		variant.ArtifactPath == sidecar.FullPath &&
		variant.Encoding == sidecar.encoding &&
		variant.Size == sidecar.Size &&
		variant.ETag != "" &&
		modTime.Equal(sidecar.ModTime)
}

func buildSidecarVariant(sidecar sidecarFile, asset *catalog.Asset) (*catalog.Variant, error) {
	return BuildSourceSidecarVariant(sidecar.File, SidecarMatch{
		AssetPath: sidecar.assetPath,
		Encoding:  sidecar.encoding,
		Suffix:    sidecar.suffix,
	}, asset)
}

func normalizedAssetPath(path, suffix string) string {
	return strings.TrimSpace(strings.TrimSuffix(path, suffix))
}

// BuildSourceSidecarVariant builds a source-sidecar variant from source and match data.
func BuildSourceSidecarVariant(
	sidecar source.File,
	match SidecarMatch,
	asset *catalog.Asset,
) (*catalog.Variant, error) {
	hash, err := pkg.HashFile(sidecar.FullPath)
	if err != nil {
		return nil, oops.In("sourcecatalog").Owner("variant").With("artifact_path", sidecar.FullPath).Wrap(err)
	}

	return &catalog.Variant{
		ID:           match.AssetPath + match.Suffix,
		AssetPath:    match.AssetPath,
		ArtifactPath: sidecar.FullPath,
		Size:         sidecar.Size,
		MediaType:    asset.MediaType,
		SourceHash:   asset.SourceHash,
		ETag:         fmt.Sprintf("%q", hash),
		Encoding:     match.Encoding,
		Metadata:     sidecarMetadata(sidecar, match.AssetPath),
	}, nil
}

func sidecarMetadata(sidecar source.File, sourcePath string) *cxmapping.Map[string, string] {
	return catalog.MetadataWithModTime(cxmapping.NewMapFrom(map[string]string{
		"stage":  SourceSidecarStage,
		"source": filepath.ToSlash(sourcePath),
	}), sidecar.ModTime)
}
