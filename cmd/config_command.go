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

func init() {
	rootCmd.AddCommand(newConfigCommand())
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
		Short: "Validate configuration without starting the server",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := loadConfigForCommand(cmd, options.files)
			if err != nil {
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
	_ = cmd
	return resolveConfigWithDix(configCommandLoadOptions(files))
}

func configCommandLoadOptions(files []string) config.LoadOptions {
	mergedFiles := append([]string(nil), configFiles...)
	mergedFiles = append(mergedFiles, files...)
	return config.LoadOptions{
		Files:   mergedFiles,
		FlagSet: configFlagSet,
	}
}
