package cmd

import (
	"context"
	"net/http"
	"time"

	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type InspectReportForTest = inspectReport

type BundleSummaryForTest = bundleSummary

func NewConfigFlagSetForTest() *pflag.FlagSet {
	return newConfigFlagSet()
}

func ConfigLoadOptionsForTest(command *cobra.Command) config.LoadOptions {
	return configLoadOptions(command)
}

func InspectAssetsForTest(ctx context.Context, cfg *config.Config) (InspectReportForTest, error) {
	return inspectAssets(ctx, cfg)
}

func ValidateConfiguredAssetsRootForTest(root string) error {
	return validateConfiguredAssetsRoot(root)
}

func EffectiveSourceInfoForTest(root string, redact bool) (map[string]any, error) {
	return effectiveSourceInfo(root, redact)
}

func CompileBundleForTest(ctx context.Context, assetsRoot, output string) (spackbundle.WriteSummary, error) {
	return compileBundle(ctx, compileOptions{
		assetsRoot: assetsRoot,
		output:     output,
		loadOptions: config.LoadOptions{
			FlagSet: newConfigFlagSet(),
		},
	})
}

func NewRuntimeRootCommandForTest() *cobra.Command {
	return newRootCommand(commandProfile{
		use:             "spack-runtime",
		short:           "Serve optimized frontend assets from a local directory or SPACK bundle.",
		enableRuntime:   true,
		enableUtilities: true,
	})
}

func NewCompilerRootCommandForTest() *cobra.Command {
	return newRootCommand(commandProfile{
		use:             "spack-compiler",
		short:           "Compile frontend assets into SPACK bundles.",
		enableCompiler:  true,
		enableUtilities: true,
	})
}

func RunHealthcheckForTest(url string, client *http.Client) error {
	return runHealthcheck(context.Background(), healthcheckOptions{
		url:     url,
		timeout: time.Second,
		client:  client,
	})
}
