package mapx

import (
	"cmp"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
)

type Entry[K cmp.Ordered, V any] struct {
	Key   K
	Value V
}

func SortedKeys[K cmp.Ordered, V any](values *cxmapping.Map[K, V]) *cxlist.List[K] {
	if values == nil {
		return cxlist.NewList[K]()
	}

	keys := cxlist.NewListWithCapacity[K](values.Len())
	values.ViewAll(func(items map[K]V) {
		for key := range items {
			keys.Add(key)
		}
	})
	return keys.Sort(cmp.Compare[K])
}

func SortedEntries[K cmp.Ordered, V any](values *cxmapping.Map[K, V]) *cxlist.List[Entry[K, V]] {
	if values == nil {
		return cxlist.NewList[Entry[K, V]]()
	}

	entries := cxlist.NewListWithCapacity[Entry[K, V]](values.Len())
	values.ViewAll(func(items map[K]V) {
		for key, value := range items {
			entries.Add(Entry[K, V]{Key: key, Value: value})
		}
	})
	return entries.Sort(func(left, right Entry[K, V]) int {
		return cmp.Compare(left.Key, right.Key)
	})
}
