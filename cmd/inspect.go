package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/media"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/spf13/cobra"
)

type inspectOptions struct {
	assets string
}

type inspectReport struct {
	AssetsRoot           string                        `json:"assets_root"`
	AssetCount           int                           `json:"asset_count"`
	SourceSidecarCount   int                           `json:"source_sidecar_count"`
	TotalAssetBytes      int64                         `json:"total_asset_bytes"`
	TotalSourceBytes     int64                         `json:"total_source_bytes"`
	Compression          map[string]compressionSummary `json:"compression"`
	ImageVariants        imageVariantSummary           `json:"image_variants"`
	EstimatedMemoryCache memoryCacheSummary            `json:"estimated_memory_cache"`
	PotentialIssues      []string                      `json:"potential_issues,omitempty"`
}

type compressionSummary struct {
	Count           int     `json:"count"`
	OriginalBytes   int64   `json:"original_bytes"`
	CompressedBytes int64   `json:"compressed_bytes"`
	SavedBytes      int64   `json:"saved_bytes"`
	SavingsRatio    float64 `json:"savings_ratio"`
}

type imageVariantSummary struct {
	Enabled                    bool     `json:"enabled"`
	SourceImageAssets          int      `json:"source_image_assets"`
	ConfiguredWidths           []int    `json:"configured_widths"`
	ConfiguredFormats          []string `json:"configured_formats"`
	PotentialGeneratedVariants int      `json:"potential_generated_variants"`
}

type memoryCacheSummary struct {
	Enabled            bool  `json:"enabled"`
	MaxBytes           int64 `json:"max_bytes"`
	EligibleBytes      int64 `json:"eligible_bytes"`
	EstimatedWarmBytes int64 `json:"estimated_warm_bytes"`
}

func init() {
	rootCmd.AddCommand(newInspectCommand())
}

func newInspectCommand() *cobra.Command {
	options := inspectOptions{}
	command := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect an asset directory without starting the server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolveConfigWithDix(configCommandLoadOptions(nil))
			if err != nil {
				return err
			}
			if strings.TrimSpace(options.assets) != "" {
				cfg.Assets.Root = options.assets
			}
			report, err := inspectAssets(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			if err := encoder.Encode(report); err != nil {
				return fmt.Errorf("encode inspect report: %w", err)
			}
			return nil
		},
	}
	command.Flags().StringVar(&options.assets, "assets", "", "Asset directory to inspect.")
	return command
}

func inspectAssets(ctx context.Context, cfg *config.Config) (inspectReport, error) {
	if strings.TrimSpace(cfg.Assets.Root) == "" {
		return inspectReport{}, errors.New("assets root is required; pass --assets or configure assets.root")
	}
	scanner, err := resolveScannerWithDix(cfg)
	if err != nil {
		return inspectReport{}, err
	}
	snapshot, err := scanner.Scan(ctx)
	if err != nil {
		return inspectReport{}, fmt.Errorf("scan assets: %w", err)
	}

	report := inspectReport{
		AssetsRoot:           filepath.Clean(cfg.Assets.Root),
		AssetCount:           snapshot.Assets.Len(),
		SourceSidecarCount:   snapshot.Variants.Len(),
		TotalSourceBytes:     snapshot.TotalBytes,
		Compression:          map[string]compressionSummary{},
		ImageVariants:        inspectImageVariants(cfg.Image, snapshot),
		EstimatedMemoryCache: inspectMemoryCache(cfg.HTTP.MemoryCache, snapshot),
		PotentialIssues:      inspectPotentialIssues(cfg, snapshot),
	}
	report.TotalAssetBytes = sumAssetBytes(snapshot)
	report.Compression = inspectCompression(snapshot)
	return report, nil
}

func sumAssetBytes(snapshot sourcecatalog.Snapshot) int64 {
	total := int64(0)
	snapshot.Assets.Range(func(_ string, asset *catalog.Asset) bool {
		if asset != nil {
			total += asset.Size
		}
		return true
	})
	return total
}

func inspectCompression(snapshot sourcecatalog.Snapshot) map[string]compressionSummary {
	out := map[string]compressionSummary{}
	snapshot.Variants.Range(func(_ string, variant *catalog.Variant) bool {
		if variant == nil || strings.TrimSpace(variant.Encoding) == "" {
			return true
		}
		asset, ok := snapshot.Assets.Get(variant.AssetPath)
		if !ok || asset == nil {
			return true
		}
		summary := out[variant.Encoding]
		summary.Count++
		summary.OriginalBytes += asset.Size
		summary.CompressedBytes += variant.Size
		out[variant.Encoding] = summary
		return true
	})
	for encoding, summary := range out {
		summary.SavedBytes = max(summary.OriginalBytes-summary.CompressedBytes, 0)
		if summary.OriginalBytes > 0 {
			summary.SavingsRatio = float64(summary.SavedBytes) / float64(summary.OriginalBytes)
		}
		out[encoding] = summary
	}
	return out
}

func inspectImageVariants(cfg config.Image, snapshot sourcecatalog.Snapshot) imageVariantSummary {
	widths := cfg.ParsedWidths().Values()
	formats := media.NormalizeImageFormats(cfg.ParsedFormats()).Values()
	imageAssets := 0
	snapshot.Assets.Range(func(_ string, asset *catalog.Asset) bool {
		if asset != nil {
			if _, ok := media.LookupImageDescriptorByMediaType(asset.MediaType); ok {
				imageAssets++
			}
		}
		return true
	})
	formatCount := max(len(formats), 1)
	return imageVariantSummary{
		Enabled:                    cfg.Enable,
		SourceImageAssets:          imageAssets,
		ConfiguredWidths:           widths,
		ConfiguredFormats:          formats,
		PotentialGeneratedVariants: boolToInt(cfg.Enable) * imageAssets * max(len(widths), 1) * formatCount,
	}
}

func inspectMemoryCache(cfg config.MemoryCache, snapshot sourcecatalog.Snapshot) memoryCacheSummary {
	eligible := int64(0)
	snapshot.Assets.Range(func(_ string, asset *catalog.Asset) bool {
		if asset != nil && asset.Size <= cfg.MaxFileSize {
			eligible += asset.Size
		}
		return true
	})
	snapshot.Variants.Range(func(_ string, variant *catalog.Variant) bool {
		if variant != nil && variant.Size <= cfg.MaxFileSize {
			eligible += variant.Size
		}
		return true
	})
	maxBytes := cfg.MaxCost()
	return memoryCacheSummary{
		Enabled:            cfg.Enabled(),
		MaxBytes:           maxBytes,
		EligibleBytes:      eligible,
		EstimatedWarmBytes: minPositive(eligible, maxBytes),
	}
}

func inspectPotentialIssues(cfg *config.Config, snapshot sourcecatalog.Snapshot) []string {
	var issues []string
	if _, ok := snapshot.Assets.Get(cfg.Assets.Entry); !ok {
		issues = append(issues, fmt.Sprintf("assets.entry %q was not found", cfg.Assets.Entry))
	}
	if strings.TrimSpace(cfg.Assets.Fallback.Target) != "" {
		if _, ok := snapshot.Assets.Get(cfg.Assets.Fallback.Target); !ok {
			issues = append(issues, fmt.Sprintf("assets.fallback.target %q was not found", cfg.Assets.Fallback.Target))
		}
	}
	if cfg.HTTP.MemoryCache.Enabled() && int64(snapshot.Assets.Len()) > int64(cfg.HTTP.MemoryCache.MaxEntries) {
		issues = append(issues, "asset count exceeds http.memory_cache.max_entries")
	}
	if cfg.Compression.PipelineEnabled() && cfg.Compression.QueueCapacity() < cfg.Async.NormalizedWorkers() {
		issues = append(issues, "compression queue capacity is smaller than normalized async workers")
	}
	if cfg.Logger.File.Enabled && strings.TrimSpace(cfg.Logger.File.Path) == "" {
		issues = append(issues, "logger.file.enabled is true but logger.file.path is empty")
	}
	return issues
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func minPositive(left, right int64) int64 {
	if right <= 0 {
		return 0
	}
	return min(left, right)
}
