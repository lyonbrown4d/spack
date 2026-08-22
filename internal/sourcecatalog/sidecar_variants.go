package sourcecatalog

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/mapx"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/pkg"
	"github.com/samber/mo"
	"github.com/samber/oops"
)

type sourceSidecarTrustFunc func(sidecarFile) (bool, error)

func (s Scanner) buildSidecarVariants(
	ctx context.Context,
	sidecars *cxmapping.Map[string, sidecarFile],
	assets *cxmapping.Map[string, *catalog.Asset],
	existingSidecars *cxmapping.Map[string, *catalog.Variant],
) (*cxmapping.Map[string, *catalog.Variant], error) {
	return buildSidecarVariants(ctx, s.trustedSourceSidecarPayload, sidecars, assets, existingSidecars)
}

func buildSidecarVariants(
	ctx context.Context,
	trustPayload sourceSidecarTrustFunc,
	sidecars *cxmapping.Map[string, sidecarFile],
	assets *cxmapping.Map[string, *catalog.Asset],
	existingSidecars *cxmapping.Map[string, *catalog.Variant],
) (*cxmapping.Map[string, *catalog.Variant], error) {
	variants, candidates, err := collectSidecarVariantBuildCandidates(sidecars, assets, existingSidecars, trustPayload)
	if err != nil {
		return nil, err
	}
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

	if err := runSourceBuildIndexes(ctx, candidates.Len(), "sourcecatalog_sidecar_build", func(runCtx context.Context, index int) error {
		if err := scanContextErr(runCtx); err != nil {
			return err
		}
		candidate, ok := candidates.Get(index)
		if !ok {
			return nil
		}
		variant, err := buildSidecarVariant(candidate.sidecar, candidate.asset)
		if err != nil {
			return err
		}
		results.Set(index, variant)
		return nil
	}); err != nil {
		return nil, oops.In("sourcecatalog").Owner("sidecar build").Wrap(err)
	}
	return results, nil
}

func publishSidecarVariants(results *cxlist.List[*catalog.Variant], variants *cxmapping.Map[string, *catalog.Variant]) {
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
	trustPayload sourceSidecarTrustFunc,
) (*cxmapping.Map[string, *catalog.Variant], *cxlist.List[sidecarVariantBuildCandidate], error) {
	variants := cxmapping.NewMapWithCapacity[string, *catalog.Variant](sidecars.Len())
	candidates := cxlist.NewListWithCapacity[sidecarVariantBuildCandidate](sidecars.Len())

	var collectErr error
	mapx.SortedKeys(sidecars).Range(func(_ int, sidecarPath string) bool {
		sidecar, _ := sidecars.Get(sidecarPath)
		asset, ok := assets.GetOption(sidecar.assetPath).Get()
		if !ok || asset == nil {
			return true
		}
		trusted, err := trustedSourceSidecarPayload(trustPayload, sidecar)
		if err != nil {
			collectErr = err
			return false
		}
		if !trusted {
			return true
		}
		if variant, ok := reusableSidecarVariant(existingSidecars, sidecar, asset).Get(); ok {
			variants.Set(variant.ID, variant)
			return true
		}
		candidates.Add(sidecarVariantBuildCandidate{sidecar: sidecar, asset: asset})
		return true
	})
	if collectErr != nil {
		return nil, nil, collectErr
	}
	return variants, candidates, nil
}

func trustedSourceSidecarPayload(trustPayload sourceSidecarTrustFunc, sidecar sidecarFile) (bool, error) {
	if trustPayload == nil {
		return false, nil
	}
	return trustPayload(sidecar)
}

func (s Scanner) trustedSourceSidecarPayload(sidecar sidecarFile) (bool, error) {
	if s.src == nil || sidecar.Size <= 0 {
		return false, nil
	}
	payload, found, err := s.src.ReadPrefix(sidecar.Path, contentcoding.ValidationSampleBytes)
	if err != nil {
		return false, oops.In("sourcecatalog").Owner("sidecar validation").With("sidecar_path", sidecar.Path).Wrap(err)
	}
	if !found {
		return false, nil
	}
	return trustedEncodedSidecarPayload(sidecar, payload), nil
}

func trustedEncodedSidecarPayload(sidecar sidecarFile, payload []byte) bool {
	if !contentcoding.IsValidPayload(sidecar.encoding, payload) {
		return false
	}
	if !pkg.RequiresMagicValidation(sidecar.assetPath) {
		return true
	}
	decoded, ok := contentcoding.DecodePayloadPrefix(sidecar.encoding, payload, 512)
	if !ok {
		return false
	}
	return pkg.HasMatchingMagic(sidecar.assetPath, decoded)
}

func (s Scanner) BuildSourceSidecarVariant(
	file source.File,
	match SidecarMatch,
	asset *catalog.Asset,
) (*catalog.Variant, bool, error) {
	sidecar := sidecarFile{
		File:      file,
		assetPath: match.AssetPath,
		encoding:  match.Encoding,
		suffix:    match.Suffix,
	}
	trusted, err := s.trustedSourceSidecarPayload(sidecar)
	if err != nil {
		return nil, false, err
	}
	if !trusted {
		return nil, false, nil
	}
	variant, err := BuildSourceSidecarVariant(file, match, asset)
	if err != nil {
		return nil, false, err
	}
	return variant, true, nil
}

func reusableSidecarVariant(
	existingSidecars *cxmapping.Map[string, *catalog.Variant],
	sidecar sidecarFile,
	asset *catalog.Asset,
) mo.Option[*catalog.Variant] {
	variant, ok := existingSidecars.GetOption(sidecar.FullPath).Get()
	if !ok || !canReuseSidecarVariant(variant, sidecar, asset) {
		return mo.None[*catalog.Variant]()
	}
	updateReusableSidecarVariant(variant, sidecar, asset)
	return mo.Some(variant)
}

func updateReusableSidecarVariant(variant *catalog.Variant, sidecar sidecarFile, asset *catalog.Asset) {
	variant.ID = sidecar.assetPath + sidecar.suffix
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
	etag, err := sidecarETag(sidecar)
	if err != nil {
		return nil, err
	}

	return &catalog.Variant{
		ID:           match.AssetPath + match.Suffix,
		AssetPath:    match.AssetPath,
		ArtifactPath: sidecar.FullPath,
		Size:         sidecar.Size,
		MediaType:    asset.MediaType,
		SourceHash:   asset.SourceHash,
		ETag:         etag,
		Encoding:     match.Encoding,
		Metadata:     sidecarMetadata(sidecar, match.AssetPath),
	}, nil
}

func sidecarETag(sidecar source.File) (string, error) {
	if etag := strings.TrimSpace(sidecar.ETag); etag != "" {
		return etag, nil
	}
	hash, err := pkg.HashFile(sidecar.FullPath)
	if err != nil {
		return "", oops.In("sourcecatalog").Owner("variant").With("artifact_path", sidecar.FullPath).Wrap(err)
	}
	return fmt.Sprintf("%q", hash), nil
}

func sidecarMetadata(sidecar source.File, sourcePath string) *cxmapping.Map[string, string] {
	return catalog.MetadataWithModTime(cxmapping.NewMapFrom(map[string]string{
		"stage":  SourceSidecarStage,
		"source": filepath.ToSlash(sourcePath),
	}), sidecar.ModTime)
}
