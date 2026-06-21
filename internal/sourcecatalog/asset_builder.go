package sourcecatalog

import (
	"context"
	"fmt"
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/pkg"
	"github.com/samber/oops"
	"golang.org/x/sync/errgroup"
	"runtime"
)

const maxSourceScanBuildParallelism = 16

type assetBuildCandidate struct {
	path string
	file source.File
}

type assetBuildResult struct {
	path  string
	asset *catalog.Asset
}

func BuildAsset(file source.File) (*catalog.Asset, error) {
	sourceHash, err := sourceFileHash(file)
	if err != nil {
		return nil, err
	}
	etag := strings.TrimSpace(file.ETag)
	if etag == "" {
		etag = fmt.Sprintf("%q", sourceHash)
	}
	mediaType := strings.TrimSpace(file.MediaType)
	if mediaType == "" {
		mediaType = string(pkg.DetectMIME(file.FullPath))
	}
	return &catalog.Asset{
		Path:       file.Path,
		FullPath:   file.FullPath,
		Size:       file.Size,
		MediaType:  mediaType,
		SourceHash: sourceHash,
		ETag:       etag,
		Metadata:   assetMetadata(file),
	}, nil
}

func sourceFileHash(file source.File) (string, error) {
	if sourceHash := strings.TrimSpace(file.SourceHash); sourceHash != "" {
		return sourceHash, nil
	}
	sourceHash, err := pkg.HashFile(file.FullPath)
	if err != nil {
		return "", oops.In("sourcecatalog").Owner("asset").With("asset_path", file.Path).Wrap(err)
	}
	return sourceHash, nil
}

func buildAssets(
	ctx context.Context,
	filesByPath *cxmapping.Map[string, source.File],
	sidecars *cxmapping.Map[string, sidecarFile],
	existingAssets *cxmapping.Map[string, *catalog.Asset],
) (*cxmapping.Map[string, *catalog.Asset], error) {
	assets, candidates := collectAssetBuildCandidates(filesByPath, sidecars, existingAssets)
	if candidates.IsEmpty() {
		return assets, nil
	}

	results := cxlist.NewConcurrentListWithCapacity[assetBuildResult](candidates.Len())
	for range candidates.Len() {
		results.Add(assetBuildResult{})
	}
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(sourceScanBuildParallelism(candidates.Len()))
	candidates.Range(func(index int, candidate assetBuildCandidate) bool {
		group.Go(func() error {
			if err := scanContextErr(groupCtx); err != nil {
				return err
			}
			asset, err := BuildAsset(candidate.file)
			if err != nil {
				return err
			}
			results.Set(index, assetBuildResult{path: candidate.path, asset: asset})
			return nil
		})
		return true
	})
	if err := group.Wait(); err != nil {
		return nil, oops.In("sourcecatalog").Owner("asset build").Wrap(err)
	}

	results.Snapshot().Range(func(_ int, result assetBuildResult) bool {
		assets.Set(result.path, result.asset)
		return true
	})
	return assets, nil
}

func collectAssetBuildCandidates(
	filesByPath *cxmapping.Map[string, source.File],
	sidecars *cxmapping.Map[string, sidecarFile],
	existingAssets *cxmapping.Map[string, *catalog.Asset],
) (*cxmapping.Map[string, *catalog.Asset], *cxlist.List[assetBuildCandidate]) {
	assets := cxmapping.NewMapWithCapacity[string, *catalog.Asset](filesByPath.Len())
	candidates := cxlist.NewListWithCapacity[assetBuildCandidate](filesByPath.Len())

	sortedKeys[source.File](filesByPath).Range(func(_ int, path string) bool {
		if sidecars.GetOption(path).IsPresent() {
			return true
		}
		file, _ := filesByPath.Get(path)
		if isExplicitBundleVariantFile(file) {
			return true
		}
		if asset, ok := existingAssets.GetOption(path).Get(); ok && canReuseAsset(asset, file) {
			asset.Metadata = catalog.MetadataWithModTime(asset.Metadata, file.ModTime)
			assets.Set(path, asset)
			return true
		}
		candidates.Add(assetBuildCandidate{path: path, file: file})
		return true
	})
	return assets, candidates
}

func canReuseAsset(asset *catalog.Asset, file source.File) bool {
	if asset == nil {
		return false
	}
	modTime, ok := catalog.MetadataModTime(asset.Metadata).Get()
	return ok &&
		asset.Path == file.Path &&
		asset.FullPath == file.FullPath &&
		asset.Size == file.Size &&
		asset.MediaType != "" &&
		asset.SourceHash != "" &&
		asset.ETag != "" &&
		modTime.Equal(file.ModTime)
}

func sourceScanBuildParallelism(total int) int {
	if total < 2 {
		return 1
	}
	return min(total, max(1, min(maxSourceScanBuildParallelism, runtime.GOMAXPROCS(0)*2)))
}

func assetMetadata(file source.File) *cxmapping.Map[string, string] {
	return catalog.MetadataWithModTime(cxmapping.NewMap[string, string](), file.ModTime)
}
