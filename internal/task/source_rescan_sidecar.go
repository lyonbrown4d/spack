package task

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
)

func (r *sourceRescanRun) reconcileSourceSidecars(
	scannedVariants *cxmapping.Map[string, *catalog.Variant],
) error {
	existingByID := r.indexSourceSidecarVariants()
	var syncErr error
	sortedMapEntries[*catalog.Variant](scannedVariants).Range(func(_ int, variant sortedMapEntry[*catalog.Variant]) bool {
		if err := r.cat.UpsertVariant(variant.value); err != nil {
			syncErr = err
			return false
		}
		existingByID.Delete(variant.key)
		return true
	})
	if syncErr != nil {
		return syncErr
	}

	sortedMapEntries[*catalog.Variant](existingByID).Range(func(_ int, variant sortedMapEntry[*catalog.Variant]) bool {
		if !r.cat.DeleteVariantByArtifactPath(variant.value.ArtifactPath) {
			return true
		}
		r.report.RemovedVariants++
		r.report.CacheInvalidations += invalidateAssetCache(r.bodyCache, variant.value.ArtifactPath)
		return true
	})
	return nil
}

func (r *sourceRescanRun) indexSourceSidecarVariants() *cxmapping.Map[string, *catalog.Variant] {
	variantsByID := cxmapping.NewMap[string, *catalog.Variant]()
	r.cat.ListVariantsByStage(sourcecatalog.SourceSidecarStage).Range(func(_ int, variant *catalog.Variant) bool {
		if sourcecatalog.IsSourceSidecarVariant(variant) {
			variantsByID.Set(variant.ID, variant)
		}
		return true
	})
	return variantsByID
}

func (r *sourceRescanRun) invalidateAssetAndVariants(
	assetPath string,
	variants *cxlist.List[*catalog.Variant],
) {
	r.report.CacheInvalidations += invalidateAssetCache(r.bodyCache, assetPath)
	r.report.RemovedVariants += r.removeAssetVariants(variants)
}

func (r *sourceRescanRun) removeAssetVariants(variants *cxlist.List[*catalog.Variant]) int {
	removed := 0
	variants.Range(func(_ int, variant *catalog.Variant) bool {
		removed++
		r.report.CacheInvalidations += invalidateAssetCache(r.bodyCache, variant.ArtifactPath)
		r.report.RemovedArtifacts += removeVariantArtifact(variant)
		return true
	})
	return removed
}

func (r *sourceRescanRun) removeSidecarVariants(match sourcecatalog.SidecarMatch) {
	if match.AssetPath == "" {
		return
	}

	variants := r.cat.ListVariants(match.AssetPath).Where(func(_ int, variant *catalog.Variant) bool {
		if variant == nil {
			return false
		}
		return variant.Encoding == match.Encoding && variant.ID == match.AssetPath+match.Suffix
	})
	if variants.IsEmpty() {
		return
	}

	variants.Range(func(_ int, variant *catalog.Variant) bool {
		if variant == nil {
			return true
		}
		if !r.cat.DeleteVariantByArtifactPath(variant.ArtifactPath) {
			return true
		}
		r.report.RemovedVariants++
		r.report.CacheInvalidations += invalidateAssetCache(r.bodyCache, variant.ArtifactPath)
		return true
	})
}
