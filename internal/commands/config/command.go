// Package configcmd implements the spack config command.
package configcmd

import (
	"github.com/arcgolabs/mapper"
	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/samber/lo"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

// Runtime contains the config command dependencies resolved by the caller.
type Runtime struct {
	Config *config.Config
	Mapper *mapper.Mapper
}

// RuntimeResolver resolves the config command runtime from CLI load options.
type RuntimeResolver func(config.LoadOptions) (Runtime, error)

type configCommandOptions struct {
	files      []string
	redact     bool
	sourceInfo bool
}

func NewCommand(resolveRuntime RuntimeResolver) *cobra.Command {
	command := &cobra.Command{
		Use:   "config",
		Short: "Validate and inspect SPACK configuration",
	}
	command.AddCommand(newConfigValidateCommand(resolveRuntime))
	command.AddCommand(newConfigPrintEffectiveCommand(resolveRuntime))
	return command
}

func newConfigValidateCommand(resolveRuntime RuntimeResolver) *cobra.Command {
	options := configCommandOptions{}
	command := &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration and asset source without starting the server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfigForCommand(cmd, options.files, resolveRuntime)
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

func newConfigPrintEffectiveCommand(resolveRuntime RuntimeResolver) *cobra.Command {
	options := configCommandOptions{}
	command := &cobra.Command{
		Use:   "print-effective",
		Short: "Print the effective merged configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigPrintEffectiveCommand(cmd, options, resolveRuntime)
		},
	}
	command.Flags().StringSliceVar(&options.files, "file", nil, "Config file path(s). Later files override earlier ones.")
	command.Flags().BoolVar(&options.redact, "redact", false, "Redact local filesystem paths.")
	command.Flags().BoolVar(&options.sourceInfo, "source-info", false, "Resolve assets.root and include source metadata.")
	return command
}

func runConfigPrintEffectiveCommand(cmd *cobra.Command, options configCommandOptions, resolveRuntime RuntimeResolver) error {
	rt, err := loadConfigRuntimeForCommand(cmd, options.files, resolveRuntime)
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

func loadConfigForCommand(cmd *cobra.Command, files []string, resolveRuntime RuntimeResolver) (*config.Config, error) {
	rt, err := loadConfigRuntimeForCommand(cmd, files, resolveRuntime)
	if err != nil {
		return nil, err
	}
	return rt.Config, nil
}

func loadConfigRuntimeForCommand(cmd *cobra.Command, files []string, resolveRuntime RuntimeResolver) (Runtime, error) {
	if resolveRuntime == nil {
		return Runtime{}, oops.In("config").Owner("runtime").Errorf("config runtime resolver is required")
	}
	rt, err := resolveRuntime(configCommandLoadOptions(cmd, files))
	if err != nil {
		return Runtime{}, oops.Wrapf(err, "resolve config")
	}
	return rt, nil
}

func configCommandLoadOptions(cmd *cobra.Command, files []string) config.LoadOptions {
	loadOptions := cmdkit.ConfigLoadOptions(cmd)
	loadOptions.Files = lo.Concat(loadOptions.Files, files)
	return loadOptions
}
