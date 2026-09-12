package main

import (
	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

func execute() error {
	command, err := newRootCommand()
	if err != nil {
		return oops.Wrapf(err, "build spack-compiler command")
	}
	if err := cmdkit.Execute(command); err != nil {
		return oops.Wrapf(err, "execute spack-compiler")
	}
	return nil
}

func newRootCommand() (*cobra.Command, error) {
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
	if err := cmdkit.BindConfigFlags(command); err != nil {
		return nil, oops.Wrapf(err, "bind compiler config flags")
	}
	command.AddCommand(
		newConfigCommand(),
		newInspectCommand(),
		newCompileCommand(),
		newVerifyCommand(),
		newDecompileCommand(),
	)
	return command, nil
}
