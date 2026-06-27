package main

import (
	compilecmd "github.com/lyonbrown4d/spack/internal/commands/compile"
	"github.com/spf13/cobra"
)

func newVerifyCommand() *cobra.Command {
	return compilecmd.NewVerifyCommand()
}
