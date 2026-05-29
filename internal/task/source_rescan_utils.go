package task

import (
	"cmp"
	"os"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
)

func indexAssetsByPath(assets *cxlist.List[*catalog.Asset]) *cxmapping.Map[string, *catalog.Asset] {
	return cxmapping.AssociateList[*catalog.Asset, string, *catalog.Asset](assets, func(_ int, asset *catalog.Asset) (string, *catalog.Asset) {
		return asset.Path, asset
	})
}

type sortedMapEntry[T any] struct {
	key   string
	value T
}

func sortedMapEntries[T any](values *cxmapping.Map[string, T]) *cxlist.List[sortedMapEntry[T]] {
	if values == nil {
		return cxlist.NewList[sortedMapEntry[T]]()
	}

	entries := cxlist.NewListWithCapacity[sortedMapEntry[T]](values.Len())
	values.ViewAll(func(items map[string]T) {
		for key, value := range items {
			entries.Add(sortedMapEntry[T]{key: key, value: value})
		}
	})
	return entries.Sort(func(left, right sortedMapEntry[T]) int {
		return cmp.Compare(left.key, right.key)
	})
}

func totalAssetBytes(assets *cxlist.List[*catalog.Asset]) int64 {
	var total int64
	assets.Range(func(_ int, asset *catalog.Asset) bool {
		if asset != nil {
			total += asset.Size
		}
		return true
	})
	return total
}

func removeVariantArtifact(variant *catalog.Variant) int {
	if sourcecatalog.IsSourceSidecarVariant(variant) {
		return 0
	}
	return removeArtifactFile(variant.ArtifactPath)
}

func invalidateAssetCache(bodyCache *assetcache.Cache, path string) int {
	if bodyCache != nil && bodyCache.Delete(path) {
		return 1
	}
	return 0
}

func removeArtifactFile(path string) int {
	if path == "" {
		return 0
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return 0
	}
	return 1
}

func assetChanged(existing, next *catalog.Asset) bool {
	if existing == nil || next == nil {
		return existing != next
	}
	return existing.FullPath != next.FullPath ||
		existing.Size != next.Size ||
		existing.MediaType != next.MediaType ||
		existing.SourceHash != next.SourceHash ||
		existing.ETag != next.ETag
}
