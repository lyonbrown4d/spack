package main

import (
	"github.com/lyonbrown4d/spack/cmd/internal/cmdruntime"
	inspectcmd "github.com/lyonbrown4d/spack/internal/commands/inspect"
	"github.com/spf13/cobra"
)

func newInspectCommand() *cobra.Command {
	return inspectcmd.NewCommand(inspectcmd.Dependencies{ResolveConfig: cmdruntime.ResolveConfigWithDix, ResolveScanner: cmdruntime.ResolveScannerWithDix})
}
