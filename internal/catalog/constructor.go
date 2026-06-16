package catalog

import (
	"sync"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
)

type IndexedCatalog struct {
	mu           sync.RWMutex
	assetCacheMu sync.RWMutex

	assetsByPath     *cxmapping.Map[string, *assetRecord]
	sortedAssetCache *cxlist.List[*assetRecord]
	variants         *variantIndex
}

type InMemoryCatalog = IndexedCatalog

func NewCatalog() Catalog {
	return NewInMemoryCatalog()
}

func NewInMemoryCatalog() *IndexedCatalog {
	return &IndexedCatalog{
		assetsByPath: cxmapping.NewMap[string, *assetRecord](),
		variants:     newVariantIndex(),
	}
}
