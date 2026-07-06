package catalog

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
)

func (c *IndexedCatalog) UpsertAsset(asset *Asset) error {
	record := newAssetRecord(asset)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.assetsByPath.Set(record.Path, record)
	c.invalidateAssetCache()
	return nil
}

func (c *IndexedCatalog) UpsertVariant(variant *Variant) error {
	id := variant.ID
	if id == "" {
		id = defaultVariantID(variant)
	}

	record := newVariantRecord(variant, id)
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.assetsByPath.Get(record.AssetPath); !ok {
		return ErrAssetNotFound
	}

	c.variants.upsert(record)
	return nil
}

func (c *IndexedCatalog) ReplaceCatalog(input ReplaceCatalogInput) error {
	if input.Assets == nil {
		input.Assets = cxlist.NewList[*Asset]()
	}
	if input.Variants == nil {
		input.Variants = cxlist.NewList[*Variant]()
	}

	nextAssets := cxmapping.NewMapWithCapacity[string, *assetRecord](input.Assets.Len())
	nextVariants := newVariantIndex()
	input.Assets.Range(func(_ int, asset *Asset) bool {
		record := newAssetRecord(asset)
		nextAssets.Set(record.Path, record)
		return true
	})

	var replaceErr error
	input.Variants.Range(func(_ int, variant *Variant) bool {
		id := variant.ID
		if id == "" {
			id = defaultVariantID(variant)
		}
		record := newVariantRecord(variant, id)
		if _, ok := nextAssets.Get(record.AssetPath); !ok {
			replaceErr = ErrAssetNotFound
			return false
		}
		nextVariants.upsert(record)
		return true
	})
	if replaceErr != nil {
		return replaceErr
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.assetsByPath = nextAssets
	c.variants = nextVariants
	c.invalidateAssetCache()
	return nil
}

func (c *IndexedCatalog) DeleteAsset(assetPath string) *cxlist.List[*Variant] {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.assetsByPath.Delete(assetPath)
	c.invalidateAssetCache()
	return c.variants.deleteByAssetPath(assetPath)
}

func (c *IndexedCatalog) DeleteVariants(assetPath string) *cxlist.List[*Variant] {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.variants.deleteByAssetPath(assetPath)
}

func (c *IndexedCatalog) DeleteVariantByArtifactPath(artifactPath string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.variants.deleteByArtifactPath(artifactPath)
}
