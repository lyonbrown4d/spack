package sourcecatalog

import (
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/catalog"
)

type existingScanState struct {
	assets   *cxmapping.Map[string, *catalog.Asset]
	sidecars *cxmapping.Map[string, *catalog.Variant]
}

func buildExistingScanState(cat catalog.Catalog) existingScanState {
	state := existingScanState{
		assets:   cxmapping.NewMap[string, *catalog.Asset](),
		sidecars: cxmapping.NewMap[string, *catalog.Variant](),
	}
	if cat == nil {
		return state
	}

	assets := cat.AllAssets()
	state.assets = cxmapping.AssociateList(assets, func(_ int, asset *catalog.Asset) (string, *catalog.Asset) {
		return asset.Path, asset
	})
	cat.ListVariantsByStage(SourceSidecarStage).Range(func(_ int, variant *catalog.Variant) bool {
		if IsSourceSidecarVariant(variant) {
			state.sidecars.Set(variant.ArtifactPath, variant)
		}
		return true
	})
	return state
}
