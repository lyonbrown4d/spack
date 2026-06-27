package compilecmd

import (
	"context"

	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/lyonbrown4d/spack/internal/compiler"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/oops"
)

func BundleForTest(ctx context.Context, assetsRoot, output string) (spackbundle.WriteSummary, error) {
	summary, err := compiler.NewService().Compile(ctx, compiler.Options{
		AssetsRoot: assetsRoot,
		Output:     output,
		LoadOptions: config.LoadOptions{
			FlagSet: cmdkit.NewConfigFlagSet(),
		},
	})
	if err != nil {
		return spackbundle.WriteSummary{}, oops.Wrapf(err, "compile bundle for test")
	}
	return summary, nil
}
