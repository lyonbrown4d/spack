package main

import (
	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

func execute() error {
	if err := cmdkit.Execute(newRootCommand()); err != nil {
		return oops.Wrapf(err, "execute spack-compiler")
	}
	return nil
}

func newRootCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "spack-compiler",
		Short: "Compile frontend assets into SPACK bundles.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return oops.Errorf("unknown command %q", args[0])
			}
			return cmd.Help()
		},
	}
	cmdkit.BindConfigFlags(command)
	command.AddCommand(
		newConfigCommand(),
		newInspectCommand(),
		newCompileCommand(),
		newVerifyCommand(),
		newDecompileCommand(),
	)
	return command
}
