package main

import (
	healthcheckcmd "github.com/lyonbrown4d/spack/internal/commands/healthcheck"
	"github.com/spf13/cobra"
)

func newHealthcheckCommand() *cobra.Command {
	return healthcheckcmd.NewCommand()
}
