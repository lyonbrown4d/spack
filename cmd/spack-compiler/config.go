package main

import (
	configcmd "github.com/lyonbrown4d/spack/internal/commands/config"
	"github.com/spf13/cobra"
)

func newConfigCommand() *cobra.Command {
	return configcmd.NewCommand()
}
