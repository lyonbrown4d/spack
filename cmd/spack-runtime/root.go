package main

import (
	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

func execute() error {
	if err := cmdkit.Execute(newRootCommand()); err != nil {
		return oops.Wrapf(err, "execute spack-runtime")
	}
	return nil
}

func newRootCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "spack-runtime",
		Short: "Serve optimized frontend assets from a local directory or SPACK bundle.",
	}
	cmdkit.BindConfigFlags(command)
	cmdkit.BindRuntimeRoot(command)
	command.AddCommand(
		newConfigCommand(),
		newInspectCommand(),
		newHealthcheckCommand(),
	)
	return command
}
