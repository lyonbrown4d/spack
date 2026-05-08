package sourcecatalog

import (
	"cmp"
	"context"
	"fmt"
	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/daiyuang/spack/internal/catalog"
	"github.com/daiyuang/spack/internal/contentcoding"
	"github.com/daiyuang/spack/internal/source"
	"github.com/daiyuang/spack/pkg"
	"github.com/samber/mo"
	"github.com/samber/oops"
	"golang.org/x/sync/errgroup"
	"path/filepath"
	"strings"
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

func recognizeSidecars(filesByPath *cxmapping.Map[string, source.File], matchers *cxlist.List[sidecarMatcher]) *cxmapping.Map[string, sidecarFile] {
	sidecars := cxmapping.NewMapWithCapacity[string, sidecarFile](filesByPath.Len())
	sortedKeys[source.File](filesByPath).Range(func(_ int, path string) bool {
		match, ok := matchSidecar(path, filesByPath, matchers).Get()
		if !ok {
			return true
		}

		match.File = filesByPath.GetOrDefault(path, source.File{})
		sidecars.Set(match.Path, match)
		return true
	})
	return sidecars
}

func matchSidecar(path string, filesByPath *cxmapping.Map[string, source.File], matchers *cxlist.List[sidecarMatcher]) mo.Option[sidecarFile] {
	matcher, ok := cxlist.FindList[sidecarMatcher](matchers, func(_ int, matcher sidecarMatcher) bool {
		if !strings.HasSuffix(path, matcher.suffix) {
			return false
		}
		assetPath := normalizedAssetPath(path, matcher.suffix)
		if assetPath == "" || assetPath == path {
			return false
		}
		_, exists := filesByPath.Get(assetPath)
		return exists
	})
	if !ok {
		return mo.None[sidecarFile]()
	}

	return mo.Some(sidecarFile{
		assetPath: normalizedAssetPath(path, matcher.suffix),
		encoding:  matcher.encoding,
		suffix:    matcher.suffix,
	})
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

	publishSidecarVariants(pending, variants)
	return variants, nil
}

func buildSidecarVariantsForCandidates(
	ctx context.Context,
	candidates *cxlist.List[sidecarVariantBuildCandidate],
) (*cxlist.List[*catalog.Variant], error) {
	results := cxlist.NewListWithCapacity[*catalog.Variant](candidates.Len())
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
