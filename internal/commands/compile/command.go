package compilecmd

import (
	"context"
	"encoding/csv"
	"errors"
	"strings"

	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewCommand() *cobra.Command {
	var output string

	command := &cobra.Command{
		Use:   "compile <assets-dir>",
		Short: "Compile frontend assets into a SPACK bundle",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			summary, err := compileBundle(cmd.Context(), compileOptions{
				assetsRoot:  args[0],
				output:      output,
				loadOptions: cmdkit.ConfigLoadOptions(cmd),
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
	cfg, err := cmdkit.ResolveConfigWithDix(loadOptions)
	if err != nil {
		return spackbundle.WriteSummary{}, oops.Wrapf(err, "resolve compile config")
	}
	compiler, err := cmdkit.ResolveCompilerWithDix(cfg)
	if err != nil {
		return spackbundle.WriteSummary{}, oops.Wrapf(err, "resolve compiler runtime")
	}
	snapshot, err := compiler.Scanner.Scan(ctx)
	if err != nil {
		return spackbundle.WriteSummary{}, oops.Wrapf(err, "scan assets")
	}
	if upsertErr := upsertCompileSnapshot(compiler.Catalog, snapshot); upsertErr != nil {
		return spackbundle.WriteSummary{}, upsertErr
	}
	if warmErr := compiler.Pipeline.Warm(ctx); warmErr != nil {
		return spackbundle.WriteSummary{}, oops.Wrapf(warmErr, "generate bundle variants")
	}
	summary, err := spackbundle.Write(ctx, spackbundle.WriteOptions{
		Output: options.output,
		Root:   cfg.Assets.Root,
		Files:  bundleFilesFromCatalog(cfg.Assets.Root, options.output, compiler.Catalog),
	})
	if err != nil {
		return spackbundle.WriteSummary{}, oops.Wrapf(err, "write spack bundle")
	}
	return summary, nil
}

func validateCompileInput(root string) error {
	if spackbundle.IsBundlePath(root) {
		return oops.In("compile").Wrap(errors.New("compile input must be an asset directory; .spack bundles are runtime sources, not compile inputs"))
	}
	return nil
}

func compileLoadOptions(assetsRoot string, base config.LoadOptions) (config.LoadOptions, error) {
	flags, err := cloneVisitedConfigFlags(base.FlagSet)
	if err != nil {
		return config.LoadOptions{}, err
	}
	if err := flags.Set("assets.root", assetsRoot); err != nil {
		return config.LoadOptions{}, oops.Wrapf(err, "set compile assets root")
	}
	return config.LoadOptions{
		Files:   append([]string(nil), base.Files...),
		FlagSet: flags,
	}, nil
}

func cloneVisitedConfigFlags(source *pflag.FlagSet) (*pflag.FlagSet, error) {
	flags := cmdkit.NewConfigFlagSet()
	if source == nil {
		return flags, nil
	}
	var cloneErr error
	source.Visit(func(flag *pflag.Flag) {
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
