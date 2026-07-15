package pipeline

import (
	"context"
	"errors"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/samber/oops"
)

var (
	ErrVariantSkipped      = oops.In("pipeline").Owner("stage").Wrap(errors.New("variant skipped"))
	errStageContextMissing = oops.In("pipeline").Owner("stage").Wrap(errors.New("stage context is nil"))
)

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
	Execute(ctx context.Context, task Task, asset *catalog.Asset) (*catalog.Variant, error)
}

type BatchStage interface {
	Stage
	ExecuteBatch(ctx context.Context, task Task, asset *catalog.Asset) (*cxlist.List[*catalog.Variant], error)
}

func IsVariantSkipped(err error) bool {
	return errors.Is(err, ErrVariantSkipped)
}

func requireStageContext(ctx context.Context) (context.Context, error) {
	if ctx == nil {
		return nil, errStageContextMissing
	}
	return ctx, nil
}
