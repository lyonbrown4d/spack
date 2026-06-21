// Package configcmd implements the spack config command.
package configcmd

import (
	"fmt"

	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/lyonbrown4d/spack/internal/config"
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
			cfg, err := loadConfigForCommand(cmd, options.files)
			if err != nil {
				return err
			}
			effective := config.EffectiveMap(cfg, options.redact)
			if options.sourceInfo {
				sourceInfo, sourceErr := effectiveSourceInfo(cfg.Assets.Root, options.redact)
				if sourceErr != nil {
					return sourceErr
				}
				effective["source_info"] = sourceInfo
			}
			body, err := yaml.Marshal(effective)
			if err != nil {
				return fmt.Errorf("marshal effective config: %w", err)
			}
			cmd.Print(string(body))
			return nil
		},
	}
	command.Flags().StringSliceVar(&options.files, "file", nil, "Config file path(s). Later files override earlier ones.")
	command.Flags().BoolVar(&options.redact, "redact", false, "Redact local filesystem paths.")
	command.Flags().BoolVar(&options.sourceInfo, "source-info", false, "Resolve assets.root and include source metadata.")
	return command
}

func loadConfigForCommand(cmd *cobra.Command, files []string) (*config.Config, error) {
	cfg, err := cmdkit.ResolveConfigWithDix(configCommandLoadOptions(cmd, files))
	if err != nil {
		return nil, fmt.Errorf("resolve config: %w", err)
	}
	return cfg, nil
}

func configCommandLoadOptions(cmd *cobra.Command, files []string) config.LoadOptions {
	loadOptions := cmdkit.ConfigLoadOptions(cmd)
	mergedFiles := append([]string(nil), loadOptions.Files...)
	mergedFiles = append(mergedFiles, files...)
	loadOptions.Files = mergedFiles
	return loadOptions
}
