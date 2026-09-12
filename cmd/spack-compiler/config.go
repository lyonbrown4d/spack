package main

import (
	"github.com/lyonbrown4d/spack/cmd/internal/cmdruntime"
	configcmd "github.com/lyonbrown4d/spack/internal/commands/config"
	"github.com/spf13/cobra"
)

func newConfigCommand() *cobra.Command {
	runner := cmdruntime.NewUtilityRunner()
	return runner.WrapCommand(configcmd.NewCommand(runner.ResolveConfigRuntimeWithDix))
}
