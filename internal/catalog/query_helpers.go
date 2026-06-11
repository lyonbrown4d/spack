package catalog

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/hashicorp/go-memdb"
	"github.com/samber/oops"
)

func assetExists(txn *memdb.Txn, assetPath string) (bool, error) {
	_, ok, err := findAssetRecord(txn, assetPath)
	return ok, err
}

func findAssetRecord(txn *memdb.Txn, assetPath string) (*assetRecord, bool, error) {
	raw, err := txn.First(catalogAssetsTable, "id", assetPath)
	if err != nil {
		return nil, false, catalogQueryError(err)
	}
	if raw == nil {
		return nil, false, nil
	}
	return assetRecordFrom(raw)
}

func findVariantRecord(txn *memdb.Txn, index string, args ...any) (*variantRecord, bool, error) {
	raw, err := txn.First(catalogVariantsTable, index, args...)
	if err != nil {
		return nil, false, catalogQueryError(err)
	}
	if raw == nil {
		return nil, false, nil
	}
	return variantRecordFrom(raw)
}

func countRecords(txn *memdb.Txn, table string) int {
	defer txn.Abort()

	iter, err := txn.Get(table, "id")
	if err != nil {
		return 0
	}

	total := 0
	for raw := iter.Next(); raw != nil; raw = iter.Next() {
		total++
	}
	return total
}

func variantViews(txn *memdb.Txn, index string, args ...any) *cxlist.List[*Variant] {
	records, err := variantRecordsResult(txn, index, args...)
	if err != nil {
		return cxlist.NewList[*Variant]()
	}
	return cxlist.MapList[*variantRecord, *Variant](records, func(_ int, record *variantRecord) *Variant {
		return record.Variant
	})
}

func variantRecords(txn *memdb.Txn, index string, args ...any) *cxlist.List[*variantRecord] {
	records, err := variantRecordsResult(txn, index, args...)
	if err != nil {
		return cxlist.NewList[*variantRecord]()
	}
	return records
}

func variantViewsResult(txn *memdb.Txn, index string, args ...any) (*cxlist.List[*Variant], error) {
	records, err := variantRecordsResult(txn, index, args...)
	if err != nil {
		return cxlist.NewList[*Variant](), err
	}
	return cxlist.MapList[*variantRecord, *Variant](records, func(_ int, record *variantRecord) *Variant {
		return record.Variant
	}), nil
}

func variantRecordsResult(txn *memdb.Txn, index string, args ...any) (*cxlist.List[*variantRecord], error) {
	iter, err := txn.Get(catalogVariantsTable, index, args...)
	if err != nil {
		return cxlist.NewList[*variantRecord](), catalogQueryError(err)
	}

	out := cxlist.NewList[*variantRecord]()
	for raw := iter.Next(); raw != nil; raw = iter.Next() {
		record, ok, err := variantRecordFrom(raw)
		if err != nil {
			return cxlist.NewList[*variantRecord](), err
		}
		if ok {
			out.Add(record)
		}
	}
	return out, nil
}

func assetRecordFrom(raw any) (*assetRecord, bool, error) {
	record, ok := raw.(*assetRecord)
	if !ok {
		return nil, false, oops.In("catalog").Owner("asset record").Wrap(ErrRecordTypeMismatch)
	}
	return record, true, nil
}

func variantRecordFrom(raw any) (*variantRecord, bool, error) {
	record, ok := raw.(*variantRecord)
	if !ok {
		return nil, false, oops.In("catalog").Owner("variant record").Wrap(ErrRecordTypeMismatch)
	}
	return record, true, nil
}

func catalogQueryError(err error) error {
	return oops.In("catalog").Owner("query").Wrap(err)
}
