package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newCompileCommand() *cobra.Command {
	var output string

	command := &cobra.Command{
		Use:   "compile <assets-dir>",
		Short: "Compile frontend assets into a SPACK bundle",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			summary, err := compileBundle(cmd.Context(), compileOptions{
				assetsRoot:  args[0],
				output:      output,
				loadOptions: configLoadOptions(cmd),
			})
			if err != nil {
				return err
			}
			cmd.Printf("compiled %d files (%d bytes) into %s\n", summary.Files, summary.Bytes, summary.Output)
			return nil
		},
	}
	command.Flags().StringVarP(&output, "output", "o", "app.spack", "Output .spack bundle path.")

	return command
}

type compileOptions struct {
	assetsRoot  string
	output      string
	loadOptions config.LoadOptions
}

func compileBundle(ctx context.Context, options compileOptions) (spackbundle.WriteSummary, error) {
	if err := validateCompileInput(options.assetsRoot); err != nil {
		return spackbundle.WriteSummary{}, err
	}
	loadOptions, err := compileLoadOptions(options.assetsRoot, options.loadOptions)
	if err != nil {
		return spackbundle.WriteSummary{}, err
	}
	cfg, err := resolveConfigWithDix(loadOptions)
	if err != nil {
		return spackbundle.WriteSummary{}, err
	}
	compiler, err := resolveCompilerWithDix(cfg)
	if err != nil {
		return spackbundle.WriteSummary{}, err
	}
	snapshot, err := compiler.scanner.Scan(ctx)
	if err != nil {
		return spackbundle.WriteSummary{}, fmt.Errorf("scan assets: %w", err)
	}
	if upsertErr := upsertCompileSnapshot(compiler.cat, snapshot); upsertErr != nil {
		return spackbundle.WriteSummary{}, upsertErr
	}
	if warmErr := compiler.pipelineSvc.Warm(ctx); warmErr != nil {
		return spackbundle.WriteSummary{}, fmt.Errorf("generate bundle variants: %w", warmErr)
	}
	summary, err := spackbundle.Write(ctx, spackbundle.WriteOptions{
		Output: options.output,
		Root:   cfg.Assets.Root,
		Files:  bundleFilesFromCatalog(cfg.Assets.Root, options.output, compiler.cat),
	})
	if err != nil {
		return spackbundle.WriteSummary{}, fmt.Errorf("write spack bundle: %w", err)
	}
	return summary, nil
}

func validateCompileInput(root string) error {
	if spackbundle.IsBundlePath(root) {
		return errors.New("compile input must be an asset directory; .spack bundles are runtime sources, not compile inputs")
	}
	return nil
}

func compileLoadOptions(assetsRoot string, base config.LoadOptions) (config.LoadOptions, error) {
	flags, err := cloneVisitedConfigFlags(base.FlagSet)
	if err != nil {
		return config.LoadOptions{}, err
	}
	if err := flags.Set("assets.root", assetsRoot); err != nil {
		return config.LoadOptions{}, fmt.Errorf("set compile assets root: %w", err)
	}
	return config.LoadOptions{
		Files:   append([]string(nil), base.Files...),
		FlagSet: flags,
	}, nil
}

func cloneVisitedConfigFlags(source *pflag.FlagSet) (*pflag.FlagSet, error) {
	flags := newConfigFlagSet()
	if source == nil {
		return flags, nil
	}
	var cloneErr error
	source.Visit(func(flag *pflag.Flag) {
		if err := flags.Set(flag.Name, flag.Value.String()); err != nil {
			cloneErr = fmt.Errorf("clone config flag %s: %w", flag.Name, err)
		}
	})
	if cloneErr != nil {
		return nil, cloneErr
	}
	return flags, nil
}
