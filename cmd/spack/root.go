package main

import (
	"fmt"

	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/spf13/cobra"
)

func execute() error {
	if err := cmdkit.Execute(newRootCommand()); err != nil {
		return fmt.Errorf("execute spack: %w", err)
	}
	return nil
}

func newRootCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "spack",
		Short: "Serve optimized frontend assets from a local directory or SPACK bundle.",
	}
	cmdkit.BindConfigFlags(command)
	cmdkit.BindRuntimeRoot(command)
	command.AddCommand(
		newConfigCommand(),
		newInspectCommand(),
		newHealthcheckCommand(),
		newCompileCommand(),
	)
	return command
}
