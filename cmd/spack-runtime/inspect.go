package main

import (
	inspectcmd "github.com/lyonbrown4d/spack/internal/commands/inspect"
	"github.com/spf13/cobra"
)

func newInspectCommand() *cobra.Command {
	return inspectcmd.NewCommand()
}
