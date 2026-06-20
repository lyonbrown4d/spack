package cmd

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newCompileCommand() *cobra.Command {
	var output string

	command := &cobra.Command{
		Use:   "compile <assets-root>",
		Short: "Compile frontend assets into a SPACK artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			summary, err := compileBundle(cmd.Context(), compileOptions{
				assetsRoot: args[0],
				output:     output,
			})
			if err != nil {
				return err
			}
			cmd.Printf("compiled %d files (%d bytes) into %s\n", summary.Files, summary.Bytes, summary.Output)
			return nil
		},
	}
	command.Flags().StringVarP(&output, "output", "o", "app.spack", "Output .spack artifact path.")

	return command
}

type compileOptions struct {
	assetsRoot string
	output     string
}

func compileBundle(ctx context.Context, options compileOptions) (spackbundle.WriteSummary, error) {
	loadOptions, err := compileLoadOptions(options.assetsRoot)
	if err != nil {
		return spackbundle.WriteSummary{}, err
	}
	cfg, err := resolveConfigWithDix(loadOptions)
	if err != nil {
		return spackbundle.WriteSummary{}, err
	}
	scanner, err := resolveScannerWithDix(cfg)
	if err != nil {
		return spackbundle.WriteSummary{}, err
	}
	snapshot, err := scanner.Scan(ctx)
	if err != nil {
		return spackbundle.WriteSummary{}, fmt.Errorf("scan assets: %w", err)
	}
	summary, err := spackbundle.Write(ctx, spackbundle.WriteOptions{
		Output: options.output,
		Root:   cfg.Assets.Root,
		Files:  bundleFilesFromSnapshot(cfg.Assets.Root, snapshot),
	})
	if err != nil {
		return spackbundle.WriteSummary{}, fmt.Errorf("write spack bundle: %w", err)
	}
	return summary, nil
}

func compileLoadOptions(assetsRoot string) (config.LoadOptions, error) {
	flags, err := cloneVisitedConfigFlags(configFlagSet)
	if err != nil {
		return config.LoadOptions{}, err
	}
	if err := flags.Set("assets.root", assetsRoot); err != nil {
		return config.LoadOptions{}, fmt.Errorf("set compile assets root: %w", err)
	}
	return config.LoadOptions{
		Files:   append([]string(nil), configFiles...),
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

func bundleFilesFromSnapshot(root string, snapshot sourcecatalog.Snapshot) []spackbundle.File {
	files := make([]spackbundle.File, 0, snapshot.Assets.Len()+snapshot.Variants.Len())
	snapshot.Assets.Range(func(_ string, asset *catalog.Asset) bool {
		if asset == nil {
			return true
		}
		files = append(files, spackbundle.File{
			Path:       asset.Path,
			FullPath:   asset.FullPath,
			Kind:       "asset",
			Size:       asset.Size,
			MediaType:  asset.MediaType,
			SourceHash: asset.SourceHash,
			ETag:       asset.ETag,
		})
		return true
	})
	snapshot.Variants.Range(func(_ string, variant *catalog.Variant) bool {
		if variant == nil || !sourcecatalog.IsSourceSidecarVariant(variant) {
			return true
		}
		files = append(files, spackbundle.File{
			Path:       bundleVariantPath(root, variant),
			FullPath:   variant.ArtifactPath,
			Kind:       "source_sidecar",
			Size:       variant.Size,
			MediaType:  variant.MediaType,
			SourceHash: variant.SourceHash,
			ETag:       variant.ETag,
			AssetPath:  variant.AssetPath,
			Encoding:   variant.Encoding,
		})
		return true
	})
	slices.SortFunc(files, func(left, right spackbundle.File) int {
		return cmp.Compare(left.Path, right.Path)
	})
	return files
}

func bundleVariantPath(root string, variant *catalog.Variant) string {
	if variant == nil {
		return ""
	}
	if rel, err := filepath.Rel(root, variant.ArtifactPath); err == nil {
		return filepath.ToSlash(rel)
	}
	return variant.ID
}
