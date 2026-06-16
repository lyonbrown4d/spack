package catalog

import (
	"cmp"

	cxlist "github.com/arcgolabs/collectionx/list"
)

func sortedAssetRecords(records []*assetRecord) *cxlist.List[*assetRecord] {
	return cxlist.NewList(records...).Sort(func(left, right *assetRecord) int {
		return cmp.Compare(left.Path, right.Path)
	})
}

func sortedVariantRecords(records []*variantRecord) *cxlist.List[*variantRecord] {
	return cxlist.NewList(records...).Sort(func(left, right *variantRecord) int {
		return cmp.Compare(variantRecordKey(left), variantRecordKey(right))
	})
}

func (c *IndexedCatalog) sortedAssets() *cxlist.List[*assetRecord] {
	c.assetCacheMu.RLock()
	records := c.sortedAssetCache
	c.assetCacheMu.RUnlock()
	if records != nil {
		return records
	}

	sorted := sortedAssetRecords(c.assetsByPath.Values())
	c.assetCacheMu.Lock()
	defer c.assetCacheMu.Unlock()
	if c.sortedAssetCache == nil {
		c.sortedAssetCache = sorted
	}
	return c.sortedAssetCache
}

func (c *IndexedCatalog) invalidateAssetCache() {
	c.assetCacheMu.Lock()
	defer c.assetCacheMu.Unlock()
	c.sortedAssetCache = nil
}

func variantViewsFromRecords(records *cxlist.List[*variantRecord]) *cxlist.List[*Variant] {
	return cxlist.MapList[*variantRecord, *Variant](records, func(_ int, record *variantRecord) *Variant {
		return record.Variant
	})
}
