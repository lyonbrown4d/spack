package task

import (
	"cmp"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/samber/oops"
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
	if strings.TrimSpace(path) == "" {
		return 0
	}
	if err := removeFileFromParentRoot(path); err != nil && !os.IsNotExist(err) {
		return 0
	}
	return 1
}

func removeFileFromParentRoot(rawPath string) error {
	absolute, err := filepath.Abs(filepath.Clean(strings.TrimSpace(rawPath)))
	if err != nil {
		return oops.In("task").Owner("artifact removal").With("path", rawPath).Wrap(err)
	}
	parent := filepath.Dir(absolute)
	name := filepath.Base(absolute)
	if name == "." || name == string(filepath.Separator) {
		return oops.In("task").Owner("artifact removal").With("path", rawPath).Wrap(errors.New("artifact path has no file name"))
	}
	rootHandle, err := openValidatedDirectoryRoot(parent, "artifact removal")
	if err != nil {
		return err
	}
	defer discardRootClose(rootHandle)
	if err := rootHandle.Remove(name); err != nil {
		return oops.In("task").Owner("artifact removal").With("path", rawPath).Wrap(err)
	}
	return nil
}

func openValidatedDirectoryRoot(root, owner string) (*os.Root, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, oops.In("task").Owner(owner).With("root", root).Wrap(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, oops.In("task").Owner(owner).With("root", root).Wrap(errors.New("root symlink not allowed"))
	}
	if !info.IsDir() {
		return nil, oops.In("task").Owner(owner).With("root", root).Wrap(errors.New("root is not a directory"))
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, oops.In("task").Owner(owner).With("root", root).Wrap(err)
	}
	openedInfo, err := rootHandle.Stat(".")
	if err != nil {
		discardRootClose(rootHandle)
		return nil, oops.In("task").Owner(owner).With("root", root).Wrap(err)
	}
	if !sameRootFile(info, openedInfo) {
		discardRootClose(rootHandle)
		return nil, oops.In("task").Owner(owner).With("root", root).Wrap(errors.New("root was replaced"))
	}
	return rootHandle, nil
}

func sameRootFile(left, right fs.FileInfo) bool {
	return left != nil && right != nil && os.SameFile(left, right)
}

func discardRootClose(root *os.Root) {
	if root == nil {
		return
	}
	if err := root.Close(); err != nil {
		return
	}
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
