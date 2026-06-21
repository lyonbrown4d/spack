// Package inspectcmd implements the spack inspect command.
package inspectcmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/media"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/spf13/cobra"
)

type inspectOptions struct {
	assets string
}

type inspectReport struct {
	AssetsRoot           string                        `json:"assets_root"`
	SourceType           string                        `json:"source_type"`
	AssetCount           int                           `json:"asset_count"`
	SourceSidecarCount   int                           `json:"source_sidecar_count"`
	TotalAssetBytes      int64                         `json:"total_asset_bytes"`
	TotalSourceBytes     int64                         `json:"total_source_bytes"`
	Bundle               *bundleSummary                `json:"bundle,omitempty"`
	Compression          map[string]compressionSummary `json:"compression"`
	ImageVariants        imageVariantSummary           `json:"image_variants"`
	EstimatedMemoryCache memoryCacheSummary            `json:"estimated_memory_cache"`
	PotentialIssues      []string                      `json:"potential_issues,omitempty"`
}

type bundleSummary struct {
	FormatVersion       string    `json:"format_version"`
	IndexKind           string    `json:"index_kind"`
	CreatedAt           time.Time `json:"created_at"`
	FileCount           int       `json:"file_count"`
	AssetCount          int       `json:"asset_count"`
	SourceSidecarCount  int       `json:"source_sidecar_count"`
	CompressedFileCount int       `json:"compressed_file_count"`
	TotalBytes          int64     `json:"total_bytes"`
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

func NewCommand() *cobra.Command {
	options := inspectOptions{}
	command := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect an asset directory or .spack bundle without starting the server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := cmdkit.ResolveConfigWithDix(cmdkit.ConfigLoadOptions(cmd))
			if err != nil {
				return fmt.Errorf("resolve inspect config: %w", err)
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
	command.Flags().StringVar(&options.assets, "assets", "", "Asset directory or .spack bundle to inspect.")
	return command
}

func inspectAssets(ctx context.Context, cfg *config.Config) (inspectReport, error) {
	if strings.TrimSpace(cfg.Assets.Root) == "" {
		return inspectReport{}, errors.New("assets root is required; pass --assets or configure assets.root")
	}
	scanner, err := cmdkit.ResolveScannerWithDix(cfg)
	if err != nil {
		return inspectReport{}, fmt.Errorf("resolve source scanner: %w", err)
	}
	snapshot, err := scanner.Scan(ctx)
	if err != nil {
		return inspectReport{}, fmt.Errorf("scan assets: %w", err)
	}
	bundle, hasBundle, err := inspectBundle(cfg.Assets.Root)
	if err != nil {
		return inspectReport{}, err
	}
	var bundlePointer *bundleSummary
	if hasBundle {
		bundlePointer = &bundle
	}

	report := inspectReport{
		AssetsRoot:           filepath.Clean(cfg.Assets.Root),
		SourceType:           inspectSourceType(hasBundle),
		AssetCount:           snapshot.Assets.Len(),
		SourceSidecarCount:   snapshot.Variants.Len(),
		TotalSourceBytes:     snapshot.TotalBytes,
		Bundle:               bundlePointer,
		Compression:          map[string]compressionSummary{},
		ImageVariants:        inspectImageVariants(cfg.Image, snapshot),
		EstimatedMemoryCache: inspectMemoryCache(cfg.HTTP.MemoryCache, snapshot),
		PotentialIssues:      inspectPotentialIssues(cfg, snapshot),
	}
	report.TotalAssetBytes = sumAssetBytes(snapshot)
	report.Compression = inspectCompression(snapshot)
	return report, nil
}

func inspectBundle(root string) (bundleSummary, bool, error) {
	if !spackbundle.IsBundlePath(root) {
		return bundleSummary{}, false, nil
	}
	index, err := spackbundle.ReadIndex(root)
	if err != nil {
		return bundleSummary{}, false, fmt.Errorf("read bundle index: %w", err)
	}
	summary := bundleSummary{
		FormatVersion: index.APIVersion,
		IndexKind:     index.Kind,
		CreatedAt:     index.CreatedAt,
		FileCount:     len(index.Files),
	}
	for indexFile := range index.Files {
		file := index.Files[indexFile]
		summary.TotalBytes += file.Size
		switch file.Kind {
		case "asset", "":
			summary.AssetCount++
		case "source_sidecar":
			summary.SourceSidecarCount++
		}
		if strings.TrimSpace(file.Encoding) != "" {
			summary.CompressedFileCount++
		}
	}
	return summary, true, nil
}

func inspectSourceType(hasBundle bool) string {
	if hasBundle {
		return "bundle"
	}
	return "directory"
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
