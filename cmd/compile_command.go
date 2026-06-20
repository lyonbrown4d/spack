package cmd

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

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
		Files:  bundleFilesFromSnapshot(cfg.Assets.Root, options.output, snapshot),
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

func bundleFilesFromSnapshot(root, output string, snapshot sourcecatalog.Snapshot) []spackbundle.File {
	files := make([]spackbundle.File, 0, snapshot.Assets.Len()+snapshot.Variants.Len())
	excludedOutput, hasExcludedOutput := normalizedOptionalPath(output)
	snapshot.Assets.Range(func(_ string, asset *catalog.Asset) bool {
		files = appendBundleAssetFile(files, asset, excludedOutput, hasExcludedOutput)
		return true
	})
	snapshot.Variants.Range(func(_ string, variant *catalog.Variant) bool {
		files = appendBundleVariantFile(files, root, variant, excludedOutput, hasExcludedOutput)
		return true
	})
	slices.SortFunc(files, func(left, right spackbundle.File) int {
		return cmp.Compare(left.Path, right.Path)
	})
	return files
}

func appendBundleAssetFile(
	files []spackbundle.File,
	asset *catalog.Asset,
	excludedOutput string,
	hasExcludedOutput bool,
) []spackbundle.File {
	if asset == nil || shouldExcludeBundlePath(asset.FullPath, excludedOutput, hasExcludedOutput) {
		return files
	}
	return append(files, spackbundle.File{
		Path:       asset.Path,
		FullPath:   asset.FullPath,
		Kind:       "asset",
		Size:       asset.Size,
		MediaType:  asset.MediaType,
		SourceHash: asset.SourceHash,
		ETag:       asset.ETag,
	})
}

func appendBundleVariantFile(
	files []spackbundle.File,
	root string,
	variant *catalog.Variant,
	excludedOutput string,
	hasExcludedOutput bool,
) []spackbundle.File {
	if variant == nil ||
		!sourcecatalog.IsSourceSidecarVariant(variant) ||
		shouldExcludeBundlePath(variant.ArtifactPath, excludedOutput, hasExcludedOutput) {
		return files
	}
	return append(files, spackbundle.File{
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
}

func shouldExcludeBundlePath(path, excludedOutput string, hasExcludedOutput bool) bool {
	return hasExcludedOutput && sameFilesystemPath(path, excludedOutput)
}

func normalizedOptionalPath(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", false
	}
	return absolute, true
}

func sameFilesystemPath(left, right string) bool {
	left, leftOK := normalizedOptionalPath(left)
	right, rightOK := normalizedOptionalPath(right)
	if !leftOK || !rightOK {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
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
