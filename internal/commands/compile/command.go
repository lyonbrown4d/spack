// Package compilecmd implements the spack compile command.
package compilecmd

import (
	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/lyonbrown4d/spack/internal/compiler"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// RuntimeResolver builds the compiler runtime for one compile command invocation.
type RuntimeResolver func(config.LoadOptions, string) (compiler.Runtime, error)

func NewCommand(resolveRuntime RuntimeResolver) *cobra.Command {
	var output string

	command := &cobra.Command{
		Use:   "compile <assets-dir>",
		Short: "Compile frontend assets into a SPACK bundle",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if resolveRuntime == nil {
				return oops.In("compile").Owner("runtime").Errorf("compiler runtime resolver is required")
			}
			runtime, err := resolveRuntime(cmdkit.ConfigLoadOptions(cmd), args[0])
			if err != nil {
				return oops.Wrapf(err, "resolve compiler runtime")
			}
			summary, err := compiler.NewService().Compile(cmd.Context(), compiler.Options{
				AssetsRoot: args[0],
				Output:     output,
				Runtime:    runtime,
			})
			if err != nil {
				return oops.Wrapf(err, "compile assets")
			}
			cmd.Printf("compiled %d files (%d bytes) into %s\n", summary.Files, summary.Bytes, summary.Output)
			return nil
		},
	}
	command.Flags().StringVarP(&output, "output", "o", "app.spack", "Output .spack bundle path.")

	return command
}
