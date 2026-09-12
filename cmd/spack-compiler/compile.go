package main

import (
	"github.com/lyonbrown4d/spack/cmd/internal/cmdruntime"
	compilecmd "github.com/lyonbrown4d/spack/internal/commands/compile"
	"github.com/lyonbrown4d/spack/internal/compiler"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/spf13/cobra"
)

func newCompileCommand() *cobra.Command {
	runner := cmdruntime.NewUtilityRunner()
	command := compilecmd.NewCommand(func(loadOptions config.LoadOptions, assetsRoot string) (compiler.Runtime, error) {
		return resolveCompilerRuntimeWithDix(runner, loadOptions, assetsRoot)
	})
	return runner.WrapCommand(command)
}
