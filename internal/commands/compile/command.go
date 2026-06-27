// Package compilecmd implements the spack compile command.
package compilecmd

import (
	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/lyonbrown4d/spack/internal/compiler"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	var output string

	command := &cobra.Command{
		Use:   "compile <assets-dir>",
		Short: "Compile frontend assets into a SPACK bundle",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			summary, err := compiler.NewService().Compile(cmd.Context(), compiler.Options{
				AssetsRoot:  args[0],
				Output:      output,
				LoadOptions: cmdkit.ConfigLoadOptions(cmd),
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
