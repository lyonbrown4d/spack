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

func variantViewsFromRecords(records *cxlist.List[*variantRecord]) *cxlist.List[*Variant] {
	return cxlist.MapList[*variantRecord, *Variant](records, func(_ int, record *variantRecord) *Variant {
		return record.Variant
	})
}
