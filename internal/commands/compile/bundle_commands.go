package compilecmd

import (
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

func NewVerifyCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "verify <bundle>",
		Short: "Verify SPACK bundle integrity",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := spackbundle.Verify(cmd.Context(), args[0]); err != nil {
				return oops.Wrapf(err, "verify bundle %s", args[0])
			}
			cmd.Printf("verified %s\n", args[0])
			return nil
		},
	}

	return command
}

func NewDecompileCommand() *cobra.Command {
	var output string

	command := &cobra.Command{
		Use:   "decompile <bundle>",
		Short: "Decompile a SPACK bundle into a directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if output == "" {
				return oops.Errorf("output directory is required (use --output or -o)")
			}
			if err := spackbundle.Decompile(cmd.Context(), args[0], output); err != nil {
				return oops.Wrapf(err, "decompile bundle %s", args[0])
			}
			cmd.Printf("decompiled %s into %s\n", args[0], output)
			return nil
		},
	}
	command.Flags().StringVarP(&output, "output", "o", "", "Output directory for extracted bundle contents.")

	return command
}
