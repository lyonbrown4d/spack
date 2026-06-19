package pipeline

import (
	"errors"
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/catalog"
)

var ErrVariantSkipped = errors.New("variant skipped")

type Request struct {
	AssetPath          string
	PreferredEncodings *cxlist.List[string]
	PreferredFormats   *cxlist.List[string]
	PreferredWidths    *cxlist.List[int]
}

type Task struct {
	AssetPath     string
	Encoding      string
	Format        string
	Width         int
	ImageVariants *cxlist.List[ImageVariantTask]
}

type ImageVariantTask struct {
	Format string
	Width  int
}

type Stage interface {
	Name() string
	Plan(asset *catalog.Asset, request Request) *cxlist.List[Task]
	Execute(task Task, asset *catalog.Asset) (*catalog.Variant, error)
}

type BatchStage interface {
	Stage
	ExecuteBatch(task Task, asset *catalog.Asset) (*cxlist.List[*catalog.Variant], error)
}

func IsVariantSkipped(err error) bool {
	return errors.Is(err, ErrVariantSkipped)
}
