package cmd

import (
	"fmt"

	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

type configCommandOptions struct {
	files  []string
	redact bool
}

func newConfigCommand() *cobra.Command {
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
			cfg, err := loadConfigForCommand(cmd, options.files)
			if err != nil {
				return err
			}
			body, err := yaml.Marshal(config.EffectiveMap(cfg, options.redact))
			if err != nil {
				return fmt.Errorf("marshal effective config: %w", err)
			}
			cmd.Print(string(body))
			return nil
		},
	}
	command.Flags().StringSliceVar(&options.files, "file", nil, "Config file path(s). Later files override earlier ones.")
	command.Flags().BoolVar(&options.redact, "redact", false, "Redact local filesystem paths.")
	return command
}

func loadConfigForCommand(cmd *cobra.Command, files []string) (*config.Config, error) {
	return resolveConfigWithDix(configCommandLoadOptions(cmd, files))
}

func configCommandLoadOptions(cmd *cobra.Command, files []string) config.LoadOptions {
	loadOptions := configLoadOptions(cmd)
	mergedFiles := append([]string(nil), loadOptions.Files...)
	mergedFiles = append(mergedFiles, files...)
	loadOptions.Files = mergedFiles
	return loadOptions
}
