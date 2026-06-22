// Package configcmd implements the spack config command.
package configcmd

import (
	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

type configCommandOptions struct {
	files      []string
	redact     bool
	sourceInfo bool
}

func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "config",
		Short: "Validate and inspect SPACK configuration",
	}
	command.AddCommand(newConfigValidateCommand())
	command.AddCommand(newConfigPrintEffectiveCommand())
	return command
}

func newConfigValidateCommand() *cobra.Command {
	options := configCommandOptions{}
	command := &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration and asset source without starting the server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfigForCommand(cmd, options.files)
			if err != nil {
				return err
			}
			if err := validateConfiguredAssetsRoot(cfg.Assets.Root); err != nil {
				return err
			}
			cmd.Println("configuration is valid")
			return nil
		},
	}
	command.Flags().StringSliceVar(&options.files, "file", nil, "Config file path(s). Later files override earlier ones.")
	return command
}

func newConfigPrintEffectiveCommand() *cobra.Command {
	options := configCommandOptions{}
	command := &cobra.Command{
		Use:   "print-effective",
		Short: "Print the effective merged configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigPrintEffectiveCommand(cmd, options)
		},
	}
	command.Flags().StringSliceVar(&options.files, "file", nil, "Config file path(s). Later files override earlier ones.")
	command.Flags().BoolVar(&options.redact, "redact", false, "Redact local filesystem paths.")
	command.Flags().BoolVar(&options.sourceInfo, "source-info", false, "Resolve assets.root and include source metadata.")
	return command
}

func runConfigPrintEffectiveCommand(cmd *cobra.Command, options configCommandOptions) error {
	rt, err := loadConfigRuntimeForCommand(cmd, options.files)
	if err != nil {
		return err
	}
	effective, err := config.BuildEffectiveConfig(rt.Mapper, rt.Config, options.redact)
	if err != nil {
		return oops.Wrapf(err, "build effective config")
	}
	if options.sourceInfo {
		sourceInfo, sourceErr := effectiveSourceInfo(rt.Config.Assets.Root, options.redact)
		if sourceErr != nil {
			return sourceErr
		}
		effective.SourceInfo = sourceInfo
	}
	body, err := yaml.Marshal(effective)
	if err != nil {
		return oops.Wrapf(err, "marshal effective config")
	}
	cmd.Print(string(body))
	return nil
}

func loadConfigForCommand(cmd *cobra.Command, files []string) (*config.Config, error) {
	cfg, err := cmdkit.ResolveConfigWithDix(configCommandLoadOptions(cmd, files))
	if err != nil {
		return nil, oops.Wrapf(err, "resolve config")
	}
	return cfg, nil
}

func loadConfigRuntimeForCommand(cmd *cobra.Command, files []string) (cmdkit.ConfigRuntime, error) {
	rt, err := cmdkit.ResolveConfigRuntimeWithDix(configCommandLoadOptions(cmd, files))
	if err != nil {
		return cmdkit.ConfigRuntime{}, oops.Wrapf(err, "resolve config")
	}
	return rt, nil
}

func configCommandLoadOptions(cmd *cobra.Command, files []string) config.LoadOptions {
	loadOptions := cmdkit.ConfigLoadOptions(cmd)
	mergedFiles := append([]string(nil), loadOptions.Files...)
	mergedFiles = append(mergedFiles, files...)
	loadOptions.Files = mergedFiles
	return loadOptions
}
