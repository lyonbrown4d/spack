package catalog

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
)

type Asset struct {
	Path       string                         `json:"path"`
	FullPath   string                         `json:"full_path"`
	Size       int64                          `json:"size"`
	MediaType  string                         `json:"media_type"`
	SourceHash string                         `json:"source_hash"`
	ETag       string                         `json:"etag"`
	Metadata   *cxmapping.Map[string, string] `json:"metadata,omitempty"`
}

func (a *Asset) GetMetadata() *cxmapping.Map[string, string] {
	if a == nil {
		return nil
	}
	return a.Metadata
}

type Variant struct {
	ID           string                         `json:"id"`
	AssetPath    string                         `json:"asset_path"`
	ArtifactPath string                         `json:"artifact_path"`
	Size         int64                          `json:"size"`
	MediaType    string                         `json:"media_type"`
	SourceHash   string                         `json:"source_hash"`
	ETag         string                         `json:"etag"`
	Encoding     string                         `json:"encoding,omitempty"`
	Format       string                         `json:"format,omitempty"`
	Width        int                            `json:"width,omitempty"`
	Metadata     *cxmapping.Map[string, string] `json:"metadata,omitempty"`
}

func (v *Variant) GetMetadata() *cxmapping.Map[string, string] {
	if v == nil {
		return nil
	}
	return v.Metadata
}

type Entry struct {
	Asset    *Asset                 `json:"asset"`
	Variants *cxlist.List[*Variant] `json:"variants,omitempty"`
}

type Snapshot struct {
	Assets *cxlist.List[*Entry] `json:"assets"`
}

type ReplaceCatalogInput struct {
	Assets   *cxlist.List[*Asset]
	Variants *cxlist.List[*Variant]
}

type BulkReplacer interface {
	ReplaceCatalog(input ReplaceCatalogInput) error
}

type Catalog interface {
	UpsertAsset(asset *Asset) error
	UpsertVariant(variant *Variant) error
	DeleteAsset(assetPath string) *cxlist.List[*Variant]
	DeleteVariants(assetPath string) *cxlist.List[*Variant]
	DeleteVariantByArtifactPath(artifactPath string) bool
	FindAsset(assetPath string) (*Asset, bool)
	FindEncodingVariant(assetPath, encoding string) (*Variant, bool)
	FindImageVariant(assetPath, format string, width int) (*Variant, bool)
	ListVariants(assetPath string) *cxlist.List[*Variant]
	ListImageVariants(assetPath, format string) *cxlist.List[*Variant]
	ListVariantsByStage(stage string) *cxlist.List[*Variant]
	AllAssets() *cxlist.List[*Asset]
	AllVariants() *cxlist.List[*Variant]
	AssetCount() int
	VariantCount() int
	Snapshot() *Snapshot
}
