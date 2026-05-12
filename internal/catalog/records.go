package catalog

import (
	"strings"

	"github.com/lyonbrown4d/spack/internal/media"
)

type assetRecord struct {
	Path  string
	Asset *Asset
}

type variantRecord struct {
	AssetPath    string
	ID           string
	ArtifactPath string
	Encoding     string
	ImageFormat  string
	Stage        string
	Width        int
	Variant      *Variant
}

func newAssetRecord(asset *Asset) *assetRecord {
	prepared := prepareAsset(asset)
	return &assetRecord{
		Path:  prepared.Path,
		Asset: prepared,
	}
}

func newVariantRecord(variant *Variant, id string) *variantRecord {
	prepared := prepareVariant(variant)
	prepared.ID = id
	return &variantRecord{
		AssetPath:    prepared.AssetPath,
		ID:           prepared.ID,
		ArtifactPath: prepared.ArtifactPath,
		Encoding:     prepared.Encoding,
		ImageFormat:  imageFormatKey(prepared),
		Stage:        stageKey(prepared),
		Width:        prepared.Width,
		Variant:      prepared,
	}
}

func imageFormatKey(variant *Variant) string {
	if variant == nil {
		return ""
	}
	if variant.Format != "" {
		return variant.Format
	}
	return media.ImageFormat(variant.MediaType)
}

func stageKey(variant *Variant) string {
	if variant == nil || variant.Metadata == nil {
		return ""
	}
	return strings.TrimSpace(variant.Metadata.GetOrDefault("stage", ""))
}

func variantRecordKey(record *variantRecord) string {
	if record == nil {
		return ""
	}
	return record.AssetPath + "\x00" + record.ID
}
