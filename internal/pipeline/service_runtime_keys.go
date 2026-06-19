package pipeline

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"strconv"
	"strings"
)

func buildStageTaskKey(stage Stage, asset *catalog.Asset, task Task) string {
	return cxlist.NewList(
		stage.Name(),
		asset.Path,
		asset.SourceHash,
		task.Encoding,
		task.Format,
		strconv.Itoa(task.Width),
		imageVariantTaskKey(task.ImageVariants),
	).Join("|")
}

func imageVariantTaskKey(variants *cxlist.List[ImageVariantTask]) string {
	if variants == nil || variants.IsEmpty() {
		return ""
	}
	return cxlist.MapList[ImageVariantTask, string](variants, func(_ int, variant ImageVariantTask) string {
		return strings.TrimSpace(variant.Format) + ":" + strconv.Itoa(variant.Width)
	}).Join(",")
}
