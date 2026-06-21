package main

import (
	compilecmd "github.com/lyonbrown4d/spack/internal/commands/compile"
	"github.com/spf13/cobra"
)

func newCompileCommand() *cobra.Command {
	return compilecmd.NewCommand()
}
