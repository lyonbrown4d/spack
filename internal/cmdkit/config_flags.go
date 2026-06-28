package cmdkit

import (
	"encoding/csv"
	"strings"

	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/configschema"
	"github.com/samber/lo"
	"github.com/samber/oops"
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

func CloneVisitedConfigFlags(sourceFlags *pflag.FlagSet) (*pflag.FlagSet, error) {
	flags := NewConfigFlagSet()
	if sourceFlags == nil {
		return flags, nil
	}
	var cloneErr error
	sourceFlags.Visit(func(flag *pflag.Flag) {
		if flags.Lookup(flag.Name) == nil {
			return
		}
		value, err := cloneConfigFlagValue(flag)
		if err != nil {
			cloneErr = oops.Wrapf(err, "clone config flag %s", flag.Name)
			return
		}
		if err := flags.Set(flag.Name, value); err != nil {
			cloneErr = oops.Wrapf(err, "clone config flag %s", flag.Name)
		}
	})
	if cloneErr != nil {
		return nil, cloneErr
	}
	return flags, nil
}

func cloneConfigFlagValue(flag *pflag.Flag) (string, error) {
	if slice, ok := flag.Value.(pflag.SliceValue); ok {
		return encodeStringSliceFlagValue(slice.GetSlice())
	}
	return flag.Value.String(), nil
}

func encodeStringSliceFlagValue(values []string) (string, error) {
	var builder strings.Builder
	writer := csv.NewWriter(&builder)
	if err := writer.Write(values); err != nil {
		return "", oops.Wrapf(err, "write string slice flag value")
	}
	writer.Flush()
	return strings.TrimSuffix(builder.String(), "\n"), nil
}
