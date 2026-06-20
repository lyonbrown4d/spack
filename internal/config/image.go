package config

import (
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/validation"
)

type Image struct {
	Enable               bool    `koanf:"enable"`
	Widths               string  `koanf:"widths"                 validate:"omitempty,spack_widths"`
	Formats              string  `koanf:"formats"`
	JPEGQuality          int     `koanf:"jpeg_quality"           validate:"gte=1,lte=100"`
	MaxSourceBytes       int64   `koanf:"max_source_bytes"       validate:"gte=0"`
	MaxSourcePixels      int64   `koanf:"max_source_pixels"      validate:"gte=0"`
	MaxWidth             int     `koanf:"max_width"              validate:"gte=0"`
	MaxHeight            int     `koanf:"max_height"             validate:"gte=0"`
	MaxOutputVariants    int     `koanf:"max_output_variants"    validate:"gte=0"`
	MaxConcurrentSources int     `koanf:"max_concurrent_sources" validate:"gte=0"`
	MaxMemoryBytes       int64   `koanf:"max_memory_bytes"       validate:"gte=0"`
	MinSavingRatio       float64 `koanf:"min_saving_ratio"       validate:"gte=0,lte=1"`
	MinSavingBytes       int64   `koanf:"min_saving_bytes"       validate:"gte=0"`
}

func (i Image) ParsedWidths() *cxlist.List[int] {
	return validation.ParseWidths(i.Widths)
}

func (i Image) ParsedFormats() *cxlist.List[string] {
	if strings.TrimSpace(i.Formats) == "" {
		return cxlist.NewList[string]()
	}
	return cxlist.NewList[string](strings.Split(i.Formats, ",")...)
}
