package catalog

import cxlist "github.com/arcgolabs/collectionx/list"

func (c *IndexedCatalog) FindAsset(assetPath string) (*Asset, bool) {
	asset, ok, err := c.FindAssetViewResult(assetPath)
	if err != nil {
		return nil, false
	}
	return cloneAsset(asset), ok
}

func (c *IndexedCatalog) FindAssetView(assetPath string) (*Asset, bool) {
	asset, ok, err := c.FindAssetViewResult(assetPath)
	if err != nil {
		return nil, false
	}
	return asset, ok
}

func (c *IndexedCatalog) FindAssetResult(assetPath string) (*Asset, bool, error) {
	asset, ok, err := c.FindAssetViewResult(assetPath)
	return cloneAsset(asset), ok, err
}

func (c *IndexedCatalog) FindAssetViewResult(assetPath string) (*Asset, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	record, ok := c.assetsByPath.Get(assetPath)
	if !ok {
		return nil, false, nil
	}
	return record.Asset, true, nil
}

func (c *IndexedCatalog) FindEncodingVariant(assetPath, encoding string) (*Variant, bool) {
	variant, ok, err := c.FindEncodingVariantViewResult(assetPath, encoding)
	if err != nil {
		return nil, false
	}
	return cloneVariant(variant), ok
}

func (c *IndexedCatalog) FindEncodingVariantView(assetPath, encoding string) (*Variant, bool) {
	variant, ok, err := c.FindEncodingVariantViewResult(assetPath, encoding)
	if err != nil {
		return nil, false
	}
	return variant, ok
}

func (c *IndexedCatalog) FindEncodingVariantResult(assetPath, encoding string) (*Variant, bool, error) {
	variant, ok, err := c.FindEncodingVariantViewResult(assetPath, encoding)
	return cloneVariant(variant), ok, err
}

func (c *IndexedCatalog) FindEncodingVariantViewResult(assetPath, encoding string) (*Variant, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	record, ok := c.variants.findEncoding(assetPath, encoding)
	if !ok {
		return nil, false, nil
	}
	return record.Variant, true, nil
}

func (c *IndexedCatalog) FindImageVariant(assetPath, format string, width int) (*Variant, bool) {
	variant, ok, err := c.FindImageVariantViewResult(assetPath, format, width)
	if err != nil {
		return nil, false
	}
	return cloneVariant(variant), ok
}

func (c *IndexedCatalog) FindImageVariantView(assetPath, format string, width int) (*Variant, bool) {
	variant, ok, err := c.FindImageVariantViewResult(assetPath, format, width)
	if err != nil {
		return nil, false
	}
	return variant, ok
}

func (c *IndexedCatalog) FindImageVariantResult(assetPath, format string, width int) (*Variant, bool, error) {
	variant, ok, err := c.FindImageVariantViewResult(assetPath, format, width)
	return cloneVariant(variant), ok, err
}

func (c *IndexedCatalog) FindImageVariantViewResult(assetPath, format string, width int) (*Variant, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	record, ok := c.variants.findImage(assetPath, format, width)
	if !ok {
		return nil, false, nil
	}
	return record.Variant, true, nil
}

func (c *IndexedCatalog) ListVariants(assetPath string) *cxlist.List[*Variant] {
	return cloneVariants(c.ListVariantsView(assetPath))
}

func (c *IndexedCatalog) ListVariantsView(assetPath string) *cxlist.List[*Variant] {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return variantViewsFromRecords(c.variants.listByAssetPath(assetPath))
}

func (c *IndexedCatalog) ListImageVariants(assetPath, format string) *cxlist.List[*Variant] {
	variants, err := c.ListImageVariantsViewResult(assetPath, format)
	if err != nil {
		return cxlist.NewList[*Variant]()
	}
	return cloneVariants(variants)
}

func (c *IndexedCatalog) ListImageVariantsView(assetPath, format string) *cxlist.List[*Variant] {
	variants, err := c.ListImageVariantsViewResult(assetPath, format)
	if err != nil {
		return cxlist.NewList[*Variant]()
	}
	return variants
}

func (c *IndexedCatalog) ListImageVariantsViewResult(assetPath, format string) (*cxlist.List[*Variant], error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return variantViewsFromRecords(c.variants.listByImageFormat(assetPath, format)), nil
}

func (c *IndexedCatalog) ListVariantsByStage(stage string) *cxlist.List[*Variant] {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return cloneVariants(variantViewsFromRecords(c.variants.listByStage(stage)))
}

func (c *IndexedCatalog) AllAssets() *cxlist.List[*Asset] {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return cxlist.MapList[*assetRecord, *Asset](c.sortedAssets(), func(_ int, record *assetRecord) *Asset {
		return cloneAsset(record.Asset)
	})
}

func (c *IndexedCatalog) AllVariants() *cxlist.List[*Variant] {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return cloneVariantRecords(c.variants.all())
}

func (c *IndexedCatalog) AssetCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.assetsByPath.Len()
}

func (c *IndexedCatalog) VariantCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.variants.count()
}

func (c *IndexedCatalog) Snapshot() *Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries := cxlist.MapList[*assetRecord, *Entry](c.sortedAssets(), func(_ int, record *assetRecord) *Entry {
		return &Entry{
			Asset:    cloneAsset(record.Asset),
			Variants: cloneVariantRecords(c.variants.listByAssetPath(record.Path)),
		}
	})
	return &Snapshot{Assets: entries}
}
