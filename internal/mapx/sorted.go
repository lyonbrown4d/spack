package mapx

import (
	"cmp"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	cxstream "github.com/arcgolabs/collectionx/stream"
)

func SortedKeys[K cmp.Ordered, V any](values *cxmapping.Map[K, V]) *cxlist.List[K] {
	if values == nil {
		return cxlist.NewList[K]()
	}
	return cxlist.NewList(values.Keys()...).Sort(cmp.Compare[K])
}

func SortedEntries[K cmp.Ordered, V any](values *cxmapping.Map[K, V]) *cxlist.List[cxstream.Entry[K, V]] {
	if values == nil {
		return cxlist.NewList[cxstream.Entry[K, V]]()
	}

	entries := values.Stream().Sorted(func(left, right cxstream.Entry[K, V]) int {
		return cmp.Compare(left.Key, right.Key)
	})
	return cxlist.NewList(entries.ToSlice()...)
}
