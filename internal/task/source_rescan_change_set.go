package task

import (
	"path/filepath"
	"strings"

	cxmapping "github.com/arcgolabs/collectionx/mapping"
	cxset "github.com/arcgolabs/collectionx/set"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/samber/oops"
)

type sourceRescanSidecarChange struct {
	match sourcecatalog.SidecarMatch
	file  source.File
}

type sourceRescanDeletedSidecar struct {
	match sourcecatalog.SidecarMatch
	path  string
}

type sourceRescanChangeSet struct {
	deletedAssets         *cxset.Set[string]
	deletedSourceSidecars *cxmapping.Map[string, sourceRescanDeletedSidecar]
	changedAssets         *cxmapping.Map[string, source.File]
	changedSourceSidecars *cxmapping.Map[string, sourceRescanSidecarChange]
	touchedAssetPaths     *cxset.Set[string]
}

func newSourceRescanChangeSet() *sourceRescanChangeSet {
	return &sourceRescanChangeSet{
		deletedAssets:         cxset.NewSet[string](),
		deletedSourceSidecars: cxmapping.NewMap[string, sourceRescanDeletedSidecar](),
		changedAssets:         cxmapping.NewMap[string, source.File](),
		changedSourceSidecars: cxmapping.NewMap[string, sourceRescanSidecarChange](),
		touchedAssetPaths:     cxset.NewSet[string](),
	}
}

func (changes *sourceRescanChangeSet) normalizeIncrementalTargets() {
	changes.deletedAssets.Range(func(assetPath string) bool {
		changes.changedAssets.Delete(assetPath)
		return true
	})

	filteredSidecars := cxmapping.NewMap[string, sourceRescanSidecarChange]()
	changes.changedSourceSidecars.Range(func(sidecarPath string, sidecarChange sourceRescanSidecarChange) bool {
		if changes.deletedAssets.Contains(sidecarChange.match.AssetPath) {
			return true
		}
		changes.touchedAssetPaths.Add(sidecarChange.match.AssetPath)
		filteredSidecars.Set(sidecarPath, sidecarChange)
		return true
	})
	changes.changedSourceSidecars = filteredSidecars
}

func (changes *sourceRescanChangeSet) isEmpty() bool {
	return changes.deletedAssets.IsEmpty() &&
		changes.deletedSourceSidecars.IsEmpty() &&
		changes.changedAssets.IsEmpty() &&
		changes.changedSourceSidecars.IsEmpty()
}

func (changes *sourceRescanChangeSet) changedAssetMatchPath(match sourcecatalog.SidecarMatch, sidecarPath string, file source.File) {
	if match.AssetPath == "" {
		return
	}
	changes.changedSourceSidecars.Set(sidecarPath, sourceRescanSidecarChange{
		match: match,
		file:  file,
	})
	changes.touchedAssetPaths.Add(match.AssetPath)
}

func (r *sourceRescanRun) processDeletedAssets(deletedAssets *cxset.Set[string]) error {
	deletedAssets.Range(func(assetPath string) bool {
		existing, ok := r.cat.FindAsset(assetPath)
		if !ok {
			return true
		}
		r.report.Removed++
		r.invalidateAssetAndVariants(existing.FullPath, r.cat.DeleteAsset(assetPath))
		return true
	})
	return nil
}

func (r *sourceRescanRun) processDeletedSourceSidecars(deletedSourceSidecars *cxmapping.Map[string, sourceRescanDeletedSidecar]) {
	deletedSourceSidecars.Range(func(_ string, change sourceRescanDeletedSidecar) bool {
		r.removeSidecarVariants(change.match)
		return true
	})
}

func (r *sourceRescanRun) processChangedAssets(changedAssets *cxmapping.Map[string, source.File]) error {
	var processErr error
	sortedMapEntries(changedAssets).Range(func(_ int, asset sortedMapEntry[source.File]) bool {
		if asset.value.Path == "" {
			return true
		}
		if err := r.upsertAssetAndSidecars(asset.key, asset.value); err != nil {
			processErr = oops.In("task").Owner("source rescan").With("asset_path", asset.key).Wrap(err)
			return false
		}
		return true
	})
	return processErr
}

func (r *sourceRescanRun) upsertAssetAndSidecars(assetPath string, file source.File) error {
	next, buildErr := sourcecatalog.BuildAsset(file)
	if buildErr != nil {
		return oops.In("task").Owner("source rescan").With("asset_path", assetPath).Wrap(buildErr)
	}

	existing, found := r.cat.FindAsset(assetPath)
	if found && !assetChanged(existing, next) {
		return nil
	}
	if found {
		r.report.Updated++
		r.invalidateAssetAndVariants(existing.FullPath, r.cat.DeleteVariants(assetPath))
	} else {
		r.report.Added++
	}

	if err := r.cat.UpsertAsset(next); err != nil {
		return oops.In("task").Owner("source rescan").With("asset_path", assetPath).Wrap(err)
	}
	return r.rebuildSourceSidecarsForAsset(next)
}

func (r *sourceRescanRun) rebuildSourceSidecarsForAsset(asset *catalog.Asset) error {
	if asset == nil {
		return nil
	}

	var rebuildErr error
	r.scanner.SidecarMatchers().Range(func(_ int, matcher sourcecatalog.SidecarMatcher) bool {
		variant, ok, err := r.buildSidecarVariant(asset, matcher)
		if err != nil {
			rebuildErr = oops.In("task").Owner("source rescan").With("asset_path", asset.Path).Wrap(err)
			return false
		}
		if !ok {
			return true
		}
		if err := r.cat.UpsertVariant(variant); err != nil {
			rebuildErr = oops.In("task").Owner("source rescan").With("asset_path", asset.Path).Wrap(err)
			return false
		}
		return true
	})
	if rebuildErr != nil {
		return rebuildErr
	}
	return nil
}

func (r *sourceRescanRun) buildSidecarVariant(
	asset *catalog.Asset,
	matcher sourcecatalog.SidecarMatcher,
) (*catalog.Variant, bool, error) {
	sidecarPath := asset.Path + matcher.Suffix
	file, found, findErr := r.scanner.FindFile(sidecarPath)
	if findErr != nil {
		return nil, false, oops.In("task").Owner("source rescan").With("asset_path", asset.Path).With("sidecar_path", sidecarPath).Wrap(findErr)
	}
	if !found {
		return nil, false, nil
	}
	variant, trusted, buildErr := r.scanner.BuildSourceSidecarVariant(file, sourcecatalog.SidecarMatch{
		AssetPath: asset.Path,
		Encoding:  matcher.Encoding,
		Suffix:    matcher.Suffix,
	}, asset)
	if buildErr != nil {
		return nil, false, oops.In("task").Owner("source rescan").With("asset_path", asset.Path).With("sidecar_path", sidecarPath).Wrap(buildErr)
	}
	if !trusted {
		return nil, false, nil
	}
	return variant, true, nil
}

func (r *sourceRescanRun) processChangedSourceSidecars(changedSidecars *cxmapping.Map[string, sourceRescanSidecarChange]) error {
	var processErr error
	sortedMapEntries(changedSidecars).Range(func(_ int, sidecar sortedMapEntry[sourceRescanSidecarChange]) bool {
		if err := r.upsertSidecarVariant(sidecar.value); err != nil {
			processErr = oops.In("task").Owner("source rescan").With("asset_path", sidecar.value.match.AssetPath).Wrap(err)
			return false
		}
		return true
	})
	return processErr
}

func (r *sourceRescanRun) upsertSidecarVariant(change sourceRescanSidecarChange) error {
	asset, ok := r.cat.FindAsset(change.match.AssetPath)
	if !ok || asset == nil {
		return nil
	}

	variant, trusted, buildErr := r.scanner.BuildSourceSidecarVariant(change.file, change.match, asset)
	if buildErr != nil {
		return oops.In("task").Owner("source rescan").With("asset_path", change.match.AssetPath).Wrap(buildErr)
	}
	if !trusted {
		r.removeSidecarVariants(change.match)
		return nil
	}
	return oops.In("task").Owner("source rescan").With("asset_path", asset.Path).Wrap(r.cat.UpsertVariant(variant))
}

func normalizeChangePath(rawPath string) string {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return ""
	}
	return strings.TrimPrefix(filepath.ToSlash(filepath.Clean(trimmed)), "/")
}
