package cmdkit

import (
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/configschema"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func BindConfigFlags(cmd *cobra.Command) {
	flags := cmd.PersistentFlags()
	flags.StringSliceP("config", "c", nil, "Config file path(s). Later files override earlier ones.")
	flags.AddFlagSet(NewConfigFlagSet())
}

func ConfigLoadOptions(cmd *cobra.Command) config.LoadOptions {
	files, err := cmd.Flags().GetStringSlice("config")
	if err != nil {
		files = nil
	}
	return config.LoadOptions{
		Files:   lo.Clone(files),
		FlagSet: cmd.Flags(),
	}
}

func NewConfigFlagSet() *pflag.FlagSet {
	flags := pflag.NewFlagSet("config", pflag.ContinueOnError)
	configschema.RegisterFlags(flags, config.DefaultConfig())
	return flags
}
