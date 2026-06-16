package catalog

import (
	"sync"

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

	cacheMu                 sync.RWMutex
	sortedByAssetPath       *cxmapping.Map[string, *cxlist.List[*variantRecord]]
	sortedByImageFormat     *cxmapping.Map[assetImageFormatKey, *cxlist.List[*variantRecord]]
	sortedByStage           *cxmapping.Map[string, *cxlist.List[*variantRecord]]
	sortedAllVariantRecords *cxlist.List[*variantRecord]
}

func newVariantIndex() *variantIndex {
	return &variantIndex{
		byKey:       cxmapping.NewMap[string, *variantRecord](),
		byAssetPath: cxmapping.NewMultiMap[string, *variantRecord](),
		byArtifact:  cxmapping.NewMap[string, *variantRecord](),
		byEncoding:  cxmapping.NewTable[string, string, *variantRecord](),
		byImage:     cxmapping.NewTable[assetImageFormatKey, int, *variantRecord](),
		byStage:     cxmapping.NewMultiMap[string, *variantRecord](),

		sortedByAssetPath:   cxmapping.NewMap[string, *cxlist.List[*variantRecord]](),
		sortedByImageFormat: cxmapping.NewMap[assetImageFormatKey, *cxlist.List[*variantRecord]](),
		sortedByStage:       cxmapping.NewMap[string, *cxlist.List[*variantRecord]](),
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
	if records, ok := idx.cachedByAssetPath(assetPath); ok {
		return records
	}
	return idx.cacheByAssetPath(assetPath, sortedVariantRecords(idx.byAssetPath.GetCopy(assetPath)))
}

func (idx *variantIndex) listByImageFormat(assetPath, format string) *cxlist.List[*variantRecord] {
	key := assetImageFormatKey{AssetPath: assetPath, Format: format}
	if records, ok := idx.cachedByImageFormat(key); ok {
		return records
	}

	row := idx.byImage.Row(key)
	records := make([]*variantRecord, 0, len(row))
	for _, record := range row {
		records = append(records, record)
	}
	return idx.cacheByImageFormat(key, sortedVariantRecords(records))
}

func (idx *variantIndex) listByStage(stage string) *cxlist.List[*variantRecord] {
	if records, ok := idx.cachedByStage(stage); ok {
		return records
	}
	return idx.cacheByStage(stage, sortedVariantRecords(idx.byStage.GetCopy(stage)))
}

func (idx *variantIndex) all() *cxlist.List[*variantRecord] {
	if records, ok := idx.cachedAll(); ok {
		return records
	}
	return idx.cacheAll(sortedVariantRecords(idx.byKey.Values()))
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
	idx.invalidateRecordCache(record)

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

	idx.invalidateRecordCache(record)

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

func (idx *variantIndex) cachedByAssetPath(assetPath string) (*cxlist.List[*variantRecord], bool) {
	idx.cacheMu.RLock()
	defer idx.cacheMu.RUnlock()
	return idx.sortedByAssetPath.Get(assetPath)
}

func (idx *variantIndex) cacheByAssetPath(assetPath string, records *cxlist.List[*variantRecord]) *cxlist.List[*variantRecord] {
	idx.cacheMu.Lock()
	defer idx.cacheMu.Unlock()
	if cached, ok := idx.sortedByAssetPath.Get(assetPath); ok {
		return cached
	}
	idx.sortedByAssetPath.Set(assetPath, records)
	return records
}

func (idx *variantIndex) cachedByImageFormat(key assetImageFormatKey) (*cxlist.List[*variantRecord], bool) {
	idx.cacheMu.RLock()
	defer idx.cacheMu.RUnlock()
	return idx.sortedByImageFormat.Get(key)
}

func (idx *variantIndex) cacheByImageFormat(key assetImageFormatKey, records *cxlist.List[*variantRecord]) *cxlist.List[*variantRecord] {
	idx.cacheMu.Lock()
	defer idx.cacheMu.Unlock()
	if cached, ok := idx.sortedByImageFormat.Get(key); ok {
		return cached
	}
	idx.sortedByImageFormat.Set(key, records)
	return records
}

func (idx *variantIndex) cachedByStage(stage string) (*cxlist.List[*variantRecord], bool) {
	idx.cacheMu.RLock()
	defer idx.cacheMu.RUnlock()
	return idx.sortedByStage.Get(stage)
}

func (idx *variantIndex) cacheByStage(stage string, records *cxlist.List[*variantRecord]) *cxlist.List[*variantRecord] {
	idx.cacheMu.Lock()
	defer idx.cacheMu.Unlock()
	if cached, ok := idx.sortedByStage.Get(stage); ok {
		return cached
	}
	idx.sortedByStage.Set(stage, records)
	return records
}

func (idx *variantIndex) cachedAll() (*cxlist.List[*variantRecord], bool) {
	idx.cacheMu.RLock()
	defer idx.cacheMu.RUnlock()
	return idx.sortedAllVariantRecords, idx.sortedAllVariantRecords != nil
}

func (idx *variantIndex) cacheAll(records *cxlist.List[*variantRecord]) *cxlist.List[*variantRecord] {
	idx.cacheMu.Lock()
	defer idx.cacheMu.Unlock()
	if idx.sortedAllVariantRecords != nil {
		return idx.sortedAllVariantRecords
	}
	idx.sortedAllVariantRecords = records
	return records
}

func (idx *variantIndex) invalidateRecordCache(record *variantRecord) {
	if record == nil {
		return
	}

	idx.cacheMu.Lock()
	defer idx.cacheMu.Unlock()

	idx.sortedByAssetPath.Delete(record.AssetPath)
	if record.ImageFormat != "" {
		idx.sortedByImageFormat.Delete(variantImageFormatKey(record))
	}
	if record.Stage != "" {
		idx.sortedByStage.Delete(record.Stage)
	}
	idx.sortedAllVariantRecords = nil
}
