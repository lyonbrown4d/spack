package cmd

import (
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewConfigFlagSetForTest() *pflag.FlagSet {
	return newConfigFlagSet()
}

func ConfigLoadOptionsForTest(command *cobra.Command) config.LoadOptions {
	return configLoadOptions(command)
}
