package main

import (
	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

func execute() error {
	command, err := newRootCommand()
	if err != nil {
		return oops.Wrapf(err, "build spack-runtime command")
	}
	if err := cmdkit.Execute(command); err != nil {
		return oops.Wrapf(err, "execute spack-runtime")
	}
	return nil
}

func newRootCommand() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   "spack-runtime",
		Short: "Serve optimized frontend assets from a local directory or SPACK bundle.",
	}
	if err := cmdkit.BindConfigFlags(command); err != nil {
		return nil, oops.Wrapf(err, "bind runtime config flags")
	}
	bindRuntimeRoot(command)
	command.AddCommand(
		newConfigCommand(),
		newInspectCommand(),
		newHealthcheckCommand(),
	)
	return command, nil
}
