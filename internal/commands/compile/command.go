// Package compilecmd implements the spack compile command.
package compilecmd

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/lyonbrown4d/spack/internal/compiler"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// RuntimeResolver builds the compiler runtime for one compile command invocation.
type RuntimeResolver func(config.LoadOptions, string) (compiler.Runtime, error)

type compileReport struct {
	GeneratedAt        time.Time `json:"generated_at"`
	Output             string    `json:"output"`
	Files              int       `json:"files"`
	Assets             int       `json:"assets"`
	Variants           int       `json:"variants"`
	SourceSidecars     int       `json:"source_sidecars"`
	SourceBytes        int64     `json:"source_bytes"`
	BundleBytes        int64     `json:"bundle_bytes"`
	DurationMillis     int64     `json:"duration_millis"`
	DurationHuman      string    `json:"duration_human"`
	CompressionEnabled bool      `json:"compression_enabled"`
	CompressionMode    string    `json:"compression_mode"`
}

func NewCommand(resolveRuntime RuntimeResolver) *cobra.Command {
	var output string
	var reportPath string

	command := &cobra.Command{
		Use:   "compile <assets-dir>",
		Short: "Compile frontend assets into a SPACK bundle",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if resolveRuntime == nil {
				return oops.In("compile").Owner("runtime").Errorf("compiler runtime resolver is required")
			}
			runtime, err := resolveRuntime(cmdkit.ConfigLoadOptions(cmd), args[0])
			if err != nil {
				return oops.Wrapf(err, "resolve compiler runtime")
			}
			startedAt := time.Now()
			summary, err := compiler.NewService().Compile(cmd.Context(), compiler.Options{
				AssetsRoot: args[0],
				Output:     output,
				Runtime:    runtime,
			})
			if err != nil {
				return oops.Wrapf(err, "compile assets")
			}
			duration := time.Since(startedAt)
			report, err := buildCompileReport(runtime, summary, duration)
			if err != nil {
				return err
			}
			if err := writeCompileReport(reportPath, report); err != nil {
				return err
			}
			cmd.Printf(
				"compiled %d files (%d source bytes, %d bundle bytes) into %s in %s\n",
				summary.Files,
				summary.Bytes,
				report.BundleBytes,
				summary.Output,
				duration.Round(time.Millisecond),
			)
			return nil
		},
	}
	command.Flags().StringVarP(&output, "output", "o", "app.spack", "Output .spack bundle path.")
	command.Flags().StringVar(&reportPath, "report", "", "Write a JSON compiler report to the given path.")

	return command
}

func buildCompileReport(runtime compiler.Runtime, summary spackbundle.WriteSummary, duration time.Duration) (compileReport, error) {
	bundleBytes, err := bundleFileSize(summary.Output)
	if err != nil {
		return compileReport{}, err
	}
	report := compileReport{
		GeneratedAt:    time.Now().UTC(),
		Output:         summary.Output,
		Files:          summary.Files,
		SourceBytes:    summary.Bytes,
		BundleBytes:    bundleBytes,
		DurationMillis: duration.Milliseconds(),
		DurationHuman:  duration.Round(time.Millisecond).String(),
	}
	if runtime.Catalog != nil {
		report.Assets = runtime.Catalog.AssetCount()
		report.Variants = runtime.Catalog.VariantCount()
		report.SourceSidecars = runtime.Catalog.ListVariantsByStage(sourcecatalog.SourceSidecarStage).Len()
	}
	if runtime.Config != nil {
		report.CompressionEnabled = runtime.Config.Compression.Enable
		report.CompressionMode = runtime.Config.Compression.NormalizedMode()
	}
	return report, nil
}

func bundleFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, oops.Wrapf(err, "stat compiled bundle")
	}
	return info.Size(), nil
}

func writeCompileReport(path string, report compileReport) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return oops.Wrapf(err, "marshal compiler report")
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return oops.Wrapf(err, "write compiler report")
	}
	return nil
}
