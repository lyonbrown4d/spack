package catalog

import (
	"sync"

	cxmapping "github.com/arcgolabs/collectionx/mapping"
)

type IndexedCatalog struct {
	mu sync.RWMutex

	assetsByPath *cxmapping.Map[string, *assetRecord]
	variants     *variantIndex
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
