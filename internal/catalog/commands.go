package catalog

import cxlist "github.com/arcgolabs/collectionx/list"

func (c *IndexedCatalog) UpsertAsset(asset *Asset) error {
	record := newAssetRecord(asset)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.assetsByPath.Set(record.Path, record)
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

func (c *IndexedCatalog) DeleteAsset(assetPath string) *cxlist.List[*Variant] {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.assetsByPath.Delete(assetPath)
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
