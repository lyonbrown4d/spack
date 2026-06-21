package compilecmd

import (
	"context"

	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

func BundleForTest(ctx context.Context, assetsRoot, output string) (spackbundle.WriteSummary, error) {
	return compileBundle(ctx, compileOptions{
		assetsRoot: assetsRoot,
		output:     output,
		loadOptions: config.LoadOptions{
			FlagSet: cmdkit.NewConfigFlagSet(),
		},
	})
}
