package task

import (
	"cmp"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/daiyuang/spack/internal/catalog"
	"github.com/daiyuang/spack/internal/sourcecatalog"
)

func (r *sourceRescanRun) reconcileSourceSidecars(
	scannedVariants *cxmapping.Map[string, *catalog.Variant],
) error {
	existingByID := r.indexSourceSidecarVariants()
	var syncErr error
	sortedMapKeys[*catalog.Variant](scannedVariants).Range(func(_ int, variantID string) bool {
		variant, _ := scannedVariants.Get(variantID)
		if err := r.cat.UpsertVariant(variant); err != nil {
			syncErr = err
			return false
		}
		existingByID.Delete(variantID)
		return true
	})
	if syncErr != nil {
		return syncErr
	}

	cxlist.NewList[*catalog.Variant](existingByID.Values()...).Sort(func(left, right *catalog.Variant) int {
		return cmp.Compare(left.ID, right.ID)
	}).Range(func(_ int, variant *catalog.Variant) bool {
		if !r.cat.DeleteVariantByArtifactPath(variant.ArtifactPath) {
			return true
		}
		r.report.RemovedVariants++
		r.report.CacheInvalidations += invalidateAssetCache(r.bodyCache, variant.ArtifactPath)
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
