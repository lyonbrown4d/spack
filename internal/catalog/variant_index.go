package catalog

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
)

type variantIndex struct {
	byKey       *cxmapping.Map[string, *variantRecord]
	byAssetPath *cxmapping.MultiMap[string, *variantRecord]
	byArtifact  *cxmapping.Map[string, *variantRecord]
	byEncoding  *cxmapping.Table[string, string, *variantRecord]
	byImage     *cxmapping.Table[assetImageFormatKey, int, *variantRecord]
	byStage     *cxmapping.MultiMap[string, *variantRecord]
}

func newVariantIndex() *variantIndex {
	return &variantIndex{
		byKey:       cxmapping.NewMap[string, *variantRecord](),
		byAssetPath: cxmapping.NewMultiMap[string, *variantRecord](),
		byArtifact:  cxmapping.NewMap[string, *variantRecord](),
		byEncoding:  cxmapping.NewTable[string, string, *variantRecord](),
		byImage:     cxmapping.NewTable[assetImageFormatKey, int, *variantRecord](),
		byStage:     cxmapping.NewMultiMap[string, *variantRecord](),
	}
}

func (idx *variantIndex) upsert(record *variantRecord) {
	idx.collectReplacements(record).Range(func(_ string, existing *variantRecord) bool {
		idx.delete(existing)
		return true
	})
	idx.insert(record)
}

func (idx *variantIndex) deleteByAssetPath(assetPath string) *cxlist.List[*Variant] {
	records := idx.listByAssetPath(assetPath)
	removed := cloneVariantRecords(records)
	records.Range(func(_ int, record *variantRecord) bool {
		idx.delete(record)
		return true
	})
	return removed
}

func (idx *variantIndex) deleteByArtifactPath(artifactPath string) bool {
	record, ok := idx.byArtifact.Get(artifactPath)
	if !ok {
		return false
	}
	idx.delete(record)
	return true
}

func (idx *variantIndex) findEncoding(assetPath, encoding string) (*variantRecord, bool) {
	return idx.byEncoding.Get(assetPath, encoding)
}

func (idx *variantIndex) findImage(assetPath, format string, width int) (*variantRecord, bool) {
	return idx.byImage.Get(assetImageFormatKey{AssetPath: assetPath, Format: format}, width)
}

func (idx *variantIndex) listByAssetPath(assetPath string) *cxlist.List[*variantRecord] {
	return sortedVariantRecords(idx.byAssetPath.GetCopy(assetPath))
}

func (idx *variantIndex) listByImageFormat(assetPath, format string) *cxlist.List[*variantRecord] {
	row := idx.byImage.Row(assetImageFormatKey{AssetPath: assetPath, Format: format})
	records := make([]*variantRecord, 0, len(row))
	for _, record := range row {
		records = append(records, record)
	}
	return sortedVariantRecords(records)
}

func (idx *variantIndex) listByStage(stage string) *cxlist.List[*variantRecord] {
	return sortedVariantRecords(idx.byStage.GetCopy(stage))
}

func (idx *variantIndex) all() *cxlist.List[*variantRecord] {
	return sortedVariantRecords(idx.byKey.Values())
}

func (idx *variantIndex) count() int {
	return idx.byKey.Len()
}

func (idx *variantIndex) collectReplacements(record *variantRecord) *cxmapping.Map[string, *variantRecord] {
	replacements := cxmapping.NewMapWithCapacity[string, *variantRecord](4)
	existing, ok := idx.byKey.Get(variantRecordKey(record))
	addReplacement(replacements, existing, ok)
	if record.ArtifactPath != "" {
		existing, ok = idx.byArtifact.Get(record.ArtifactPath)
		addReplacement(replacements, existing, ok)
	}
	if record.Encoding != "" {
		existing, ok = idx.byEncoding.Get(record.AssetPath, record.Encoding)
		addReplacement(replacements, existing, ok)
	}
	if record.ImageFormat != "" {
		existing, ok = idx.byImage.Get(variantImageFormatKey(record), record.Width)
		addReplacement(replacements, existing, ok)
	}
	return replacements
}

func addReplacement(replacements *cxmapping.Map[string, *variantRecord], record *variantRecord, ok bool) {
	if ok {
		replacements.Set(variantRecordKey(record), record)
	}
}

func (idx *variantIndex) insert(record *variantRecord) {
	idx.byKey.Set(variantRecordKey(record), record)
	idx.byAssetPath.Put(record.AssetPath, record)
	if record.ArtifactPath != "" {
		idx.byArtifact.Set(record.ArtifactPath, record)
	}
	if record.Encoding != "" {
		idx.byEncoding.Put(record.AssetPath, record.Encoding, record)
	}
	if record.ImageFormat != "" {
		idx.byImage.Put(variantImageFormatKey(record), record.Width, record)
	}
	if record.Stage != "" {
		idx.byStage.Put(record.Stage, record)
	}
}

func (idx *variantIndex) delete(record *variantRecord) {
	if record == nil {
		return
	}

	key := variantRecordKey(record)
	idx.byKey.Delete(key)
	idx.byAssetPath.DeleteValueIf(record.AssetPath, func(existing *variantRecord) bool {
		return variantRecordKey(existing) == key
	})
	if record.ArtifactPath != "" {
		idx.byArtifact.Delete(record.ArtifactPath)
	}
	if record.Encoding != "" {
		idx.byEncoding.Delete(record.AssetPath, record.Encoding)
	}
	if record.ImageFormat != "" {
		idx.byImage.Delete(variantImageFormatKey(record), record.Width)
	}
	if record.Stage != "" {
		idx.byStage.DeleteValueIf(record.Stage, func(existing *variantRecord) bool {
			return variantRecordKey(existing) == key
		})
	}
}
