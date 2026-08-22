package sourcecatalog

import (
	"context"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/samber/oops"
)

func runSourceBuildIndexes(ctx context.Context, total int, workload string, run func(context.Context, int) error) error {
	indexes := cxlist.NewListWithCapacity[int](total)
	for index := range total {
		indexes.Add(index)
	}
	runner := asyncx.NewRunner(nil, &asyncx.Settings{Size: sourceScanBuildParallelism(total)}, workload)
	return oops.In("sourcecatalog").Owner(workload).Wrap(asyncx.RunListWith(ctx, runner, indexes, run))
}
