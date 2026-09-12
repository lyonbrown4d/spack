package main

import (
	"github.com/lyonbrown4d/spack/cmd/internal/cmdruntime"
	inspectcmd "github.com/lyonbrown4d/spack/internal/commands/inspect"
	"github.com/spf13/cobra"
)

func newInspectCommand() *cobra.Command {
	runner := cmdruntime.NewUtilityRunner()
	return runner.WrapCommand(inspectcmd.NewCommand(inspectcmd.Dependencies{
		ResolveConfig:  runner.ResolveConfigWithDix,
		ResolveScanner: runner.ResolveScannerWithDix,
	}))
}
