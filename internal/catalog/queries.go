package catalog

import cxlist "github.com/arcgolabs/collectionx/list"

func (c *MemDBCatalog) FindAsset(assetPath string) (*Asset, bool) {
	asset, ok, err := c.FindAssetViewResult(assetPath)
	if err != nil {
		return nil, false
	}
	return cloneAsset(asset), ok
}

func (c *MemDBCatalog) FindAssetView(assetPath string) (*Asset, bool) {
	asset, ok, err := c.FindAssetViewResult(assetPath)
	if err != nil {
		return nil, false
	}
	return asset, ok
}

func (c *MemDBCatalog) FindAssetResult(assetPath string) (*Asset, bool, error) {
	asset, ok, err := c.FindAssetViewResult(assetPath)
	return cloneAsset(asset), ok, err
}

func (c *MemDBCatalog) FindAssetViewResult(assetPath string) (*Asset, bool, error) {
	txn := c.db.Txn(false)
	defer txn.Abort()

	record, ok, err := findAssetRecord(txn, assetPath)
	if err != nil || !ok {
		return nil, ok, err
	}
	return record.Asset, true, nil
}

func (c *MemDBCatalog) FindEncodingVariant(assetPath, encoding string) (*Variant, bool) {
	variant, ok, err := c.FindEncodingVariantViewResult(assetPath, encoding)
	if err != nil {
		return nil, false
	}
	return cloneVariant(variant), ok
}

func (c *MemDBCatalog) FindEncodingVariantView(assetPath, encoding string) (*Variant, bool) {
	variant, ok, err := c.FindEncodingVariantViewResult(assetPath, encoding)
	if err != nil {
		return nil, false
	}
	return variant, ok
}

func (c *MemDBCatalog) FindEncodingVariantResult(assetPath, encoding string) (*Variant, bool, error) {
	variant, ok, err := c.FindEncodingVariantViewResult(assetPath, encoding)
	return cloneVariant(variant), ok, err
}

func (c *MemDBCatalog) FindEncodingVariantViewResult(assetPath, encoding string) (*Variant, bool, error) {
	txn := c.db.Txn(false)
	defer txn.Abort()

	record, ok, err := findVariantRecord(txn, catalogVariantAssetEncodingIndex, assetPath, encoding)
	if err != nil || !ok {
		return nil, ok, err
	}
	return record.Variant, true, nil
}

func (c *MemDBCatalog) FindImageVariant(assetPath, format string, width int) (*Variant, bool) {
	variant, ok, err := c.FindImageVariantViewResult(assetPath, format, width)
	if err != nil {
		return nil, false
	}
	return cloneVariant(variant), ok
}

func (c *MemDBCatalog) FindImageVariantView(assetPath, format string, width int) (*Variant, bool) {
	variant, ok, err := c.FindImageVariantViewResult(assetPath, format, width)
	if err != nil {
		return nil, false
	}
	return variant, ok
}

func (c *MemDBCatalog) FindImageVariantResult(assetPath, format string, width int) (*Variant, bool, error) {
	variant, ok, err := c.FindImageVariantViewResult(assetPath, format, width)
	return cloneVariant(variant), ok, err
}

func (c *MemDBCatalog) FindImageVariantViewResult(assetPath, format string, width int) (*Variant, bool, error) {
	txn := c.db.Txn(false)
	defer txn.Abort()

	record, ok, err := findVariantRecord(txn, catalogVariantAssetFormatWidthIndex, assetPath, format, width)
	if err != nil || !ok {
		return nil, ok, err
	}
	return record.Variant, true, nil
}

func (c *MemDBCatalog) ListVariants(assetPath string) *cxlist.List[*Variant] {
	return cloneVariants(c.ListVariantsView(assetPath))
}

func (c *MemDBCatalog) ListVariantsView(assetPath string) *cxlist.List[*Variant] {
	txn := c.db.Txn(false)
	defer txn.Abort()

	return variantViews(txn, catalogVariantAssetPathIndex, assetPath)
}

func (c *MemDBCatalog) ListImageVariants(assetPath, format string) *cxlist.List[*Variant] {
	variants, err := c.ListImageVariantsViewResult(assetPath, format)
	if err != nil {
		return cxlist.NewList[*Variant]()
	}
	return cloneVariants(variants)
}

func (c *MemDBCatalog) ListImageVariantsView(assetPath, format string) *cxlist.List[*Variant] {
	variants, err := c.ListImageVariantsViewResult(assetPath, format)
	if err != nil {
		return cxlist.NewList[*Variant]()
	}
	return variants
}

func (c *MemDBCatalog) ListImageVariantsViewResult(assetPath, format string) (*cxlist.List[*Variant], error) {
	txn := c.db.Txn(false)
	defer txn.Abort()

	return variantViewsResult(txn, catalogVariantAssetFormatWidthIndex+"_prefix", assetPath, format)
}

func (c *MemDBCatalog) ListVariantsByStage(stage string) *cxlist.List[*Variant] {
	txn := c.db.Txn(false)
	defer txn.Abort()

	return cloneVariants(variantViews(txn, catalogVariantStageIndex, stage))
}

func (c *MemDBCatalog) AllAssets() *cxlist.List[*Asset] {
	txn := c.db.Txn(false)
	defer txn.Abort()

	iter, err := txn.Get(catalogAssetsTable, "id")
	if err != nil {
		return cxlist.NewList[*Asset]()
	}

	out := cxlist.NewList[*Asset]()
	for raw := iter.Next(); raw != nil; raw = iter.Next() {
		record, ok, err := assetRecordFrom(raw)
		if err != nil {
			continue
		}
		if ok {
			out.Add(cloneAsset(record.Asset))
		}
	}
	return out
}

func (c *MemDBCatalog) AllVariants() *cxlist.List[*Variant] {
	txn := c.db.Txn(false)
	defer txn.Abort()

	return cloneVariants(variantViews(txn, "id"))
}

func (c *MemDBCatalog) AssetCount() int {
	return countRecords(c.db.Txn(false), catalogAssetsTable)
}

func (c *MemDBCatalog) VariantCount() int {
	return countRecords(c.db.Txn(false), catalogVariantsTable)
}

func (c *MemDBCatalog) Snapshot() *Snapshot {
	txn := c.db.Txn(false)
	defer txn.Abort()

	assets, err := txn.Get(catalogAssetsTable, "id")
	if err != nil {
		return &Snapshot{Assets: cxlist.NewList[*Entry]()}
	}

	entries := cxlist.NewList[*Entry]()
	for raw := assets.Next(); raw != nil; raw = assets.Next() {
		record, ok, err := assetRecordFrom(raw)
		if err != nil {
			continue
		}
		if !ok {
			continue
		}
		entries.Add(&Entry{
			Asset:    cloneAsset(record.Asset),
			Variants: cloneVariants(variantViews(txn, catalogVariantAssetPathIndex, record.Path)),
		})
	}
	return &Snapshot{Assets: entries}
}
