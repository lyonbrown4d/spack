package catalog

import (
	"errors"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/samber/oops"
)

var (
	errNilAsset   = errors.New("catalog asset is nil")
	errNilVariant = errors.New("catalog variant is nil")
)

func (c *IndexedCatalog) UpsertAsset(asset *Asset) error {
	record, err := buildAssetRecord(asset, -1)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.assetsByPath.Set(record.Path, record)
	c.invalidateAssetCache()
	return nil
}

func (c *IndexedCatalog) UpsertVariant(variant *Variant) error {
	record, err := buildVariantRecord(variant, -1)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.assetsByPath.Get(record.AssetPath); !ok {
		return ErrAssetNotFound
	}

	c.variants.upsert(record)
	return nil
}

func (c *IndexedCatalog) ReplaceCatalog(input ReplaceCatalogInput) error {
	if input.Assets == nil {
		input.Assets = cxlist.NewList[*Asset]()
	}
	if input.Variants == nil {
		input.Variants = cxlist.NewList[*Variant]()
	}

	nextAssets, err := buildAssetRecords(input.Assets)
	if err != nil {
		return err
	}
	nextVariants, err := buildVariantRecords(input.Variants, nextAssets)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.assetsByPath = nextAssets
	c.variants = nextVariants
	c.invalidateAssetCache()
	return nil
}

func buildAssetRecords(assets *cxlist.List[*Asset]) (*cxmapping.Map[string, *assetRecord], error) {
	nextAssets := cxmapping.NewMapWithCapacity[string, *assetRecord](assets.Len())
	var buildErr error
	assets.Range(func(index int, asset *Asset) bool {
		record, err := buildAssetRecord(asset, index)
		if err != nil {
			buildErr = err
			return false
		}
		nextAssets.Set(record.Path, record)
		return true
	})
	if buildErr != nil {
		return nil, buildErr
	}
	return nextAssets, nil
}

func buildVariantRecords(
	variants *cxlist.List[*Variant],
	assets *cxmapping.Map[string, *assetRecord],
) (*variantIndex, error) {
	nextVariants := newVariantIndex()
	var buildErr error
	variants.Range(func(index int, variant *Variant) bool {
		record, err := buildVariantRecord(variant, index)
		if err != nil {
			buildErr = err
			return false
		}
		if _, ok := assets.Get(record.AssetPath); !ok {
			buildErr = ErrAssetNotFound
			return false
		}
		nextVariants.upsert(record)
		return true
	})
	if buildErr != nil {
		return nil, buildErr
	}
	return nextVariants, nil
}

func buildAssetRecord(asset *Asset, index int) (*assetRecord, error) {
	if asset == nil {
		return nil, catalogInputError(errNilAsset, index)
	}
	return newAssetRecord(asset), nil
}

func buildVariantRecord(variant *Variant, index int) (*variantRecord, error) {
	if variant == nil {
		return nil, catalogInputError(errNilVariant, index)
	}
	id := variant.ID
	if id == "" {
		id = defaultVariantID(variant)
	}
	return newVariantRecord(variant, id), nil
}

func catalogInputError(err error, index int) error {
	builder := oops.In("catalog").Owner("indexed catalog")
	if index >= 0 {
		builder = builder.With("index", index)
	}
	return builder.Wrap(err)
}

func (c *IndexedCatalog) DeleteAsset(assetPath string) *cxlist.List[*Variant] {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.assetsByPath.Delete(assetPath)
	c.invalidateAssetCache()
	return c.variants.deleteByAssetPath(assetPath)
}

func (c *IndexedCatalog) DeleteVariants(assetPath string) *cxlist.List[*Variant] {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.variants.deleteByAssetPath(assetPath)
}

func (c *IndexedCatalog) DeleteVariantByArtifactPath(artifactPath string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.variants.deleteByArtifactPath(artifactPath)
}
