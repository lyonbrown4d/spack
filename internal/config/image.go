package config

import (
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/validation"
)

type Image struct {
	Enable      bool   `koanf:"enable"`
	Widths      string `koanf:"widths"       validate:"omitempty,spack_widths"`
	Formats     string `koanf:"formats"`
	JPEGQuality int    `koanf:"jpeg_quality" validate:"gte=1,lte=100"`
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
