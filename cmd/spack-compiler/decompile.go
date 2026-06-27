package main

import (
	compilecmd "github.com/lyonbrown4d/spack/internal/commands/compile"
	"github.com/spf13/cobra"
)

func newDecompileCommand() *cobra.Command {
	return compilecmd.NewDecompileCommand()
}
