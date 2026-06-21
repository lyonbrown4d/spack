// Package assetprofile estimates byte-level characteristics of static assets.
package assetprofile

import (
	"fmt"
	"math"

	"github.com/arcgolabs/collectionx/bytex"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/catalog"
)

const (
	defaultMaxSampleBytes = int64(16 << 20)
	defaultTopByteCount   = 8
	readBufferSize        = 32 * 1024
)

type Options struct {
	MaxSampleBytes int64
	TopByteCount   int
}

type Summary struct {
	AssetCount               int             `json:"asset_count"`
	ProfiledAssetCount       int             `json:"profiled_asset_count"`
	FailedAssetCount         int             `json:"failed_asset_count,omitempty"`
	SampledBytes             int64           `json:"sampled_bytes"`
	Truncated                bool            `json:"truncated"`
	UniqueBytes              int             `json:"unique_bytes"`
	EntropyBitsPerByte       float64         `json:"entropy_bits_per_byte"`
	NullByteRatio            float64         `json:"null_byte_ratio"`
	ASCIIByteRatio           float64         `json:"ascii_byte_ratio"`
	PrintableASCIIByteRatio  float64         `json:"printable_ascii_byte_ratio"`
	EstimatedCompressibility string          `json:"estimated_compressibility"`
	TopBytes                 []ByteFrequency `json:"top_bytes,omitempty"`
}

type ByteFrequency struct {
	Value byte    `json:"value"`
	Hex   string  `json:"hex"`
	Count int     `json:"count"`
	Ratio float64 `json:"ratio"`
}

func DefaultOptions() Options {
	return Options{
		MaxSampleBytes: defaultMaxSampleBytes,
		TopByteCount:   defaultTopByteCount,
	}
}

func AnalyzeAssets(assets *cxmapping.Map[string, *catalog.Asset], opts Options) Summary {
	opts = normalizeOptions(opts)
	summary := Summary{EstimatedCompressibility: "unknown"}
	if assets == nil || assets.IsEmpty() || opts.MaxSampleBytes <= 0 {
		return summary
	}

	counter := bytex.NewCounter()
	state := analysisState{
		summary:       summary,
		remaining:     opts.MaxSampleBytes,
		declaredBytes: totalDeclaredAssetBytes(assets),
	}
	assets.Range(func(_ string, asset *catalog.Asset) bool {
		state.profileAsset(asset, counter)
		return true
	})

	return finalizeSummary(state.summary, counter, opts.TopByteCount)
}

type analysisState struct {
	summary       Summary
	remaining     int64
	declaredBytes int64
}

func (s *analysisState) profileAsset(asset *catalog.Asset, counter *bytex.Counter) {
	if asset == nil {
		return
	}
	s.summary.AssetCount++
	if s.remaining <= 0 {
		s.summary.Truncated = true
		return
	}
	read, err := countAssetBytes(asset.FullPath, s.remaining, counter)
	if err != nil {
		s.summary.FailedAssetCount++
		return
	}
	s.summary.ProfiledAssetCount++
	s.summary.SampledBytes += read
	s.remaining -= read
	if s.remaining <= 0 && s.summary.SampledBytes < s.declaredBytes {
		s.summary.Truncated = true
	}
}

func normalizeOptions(opts Options) Options {
	if opts.MaxSampleBytes <= 0 {
		opts.MaxSampleBytes = defaultMaxSampleBytes
	}
	if opts.TopByteCount <= 0 {
		opts.TopByteCount = defaultTopByteCount
	}
	return opts
}

func finalizeSummary(summary Summary, counter *bytex.Counter, topByteCount int) Summary {
	total := counter.Len()
	if total == 0 {
		return summary
	}

	summary.UniqueBytes = counter.UniqueLen()
	summary.NullByteRatio = ratio(counter.Count(0), total)
	summary.ASCIIByteRatio = ratio(countRange(counter, 0, 128), total)
	summary.PrintableASCIIByteRatio = ratio(countRange(counter, 32, 127), total)
	summary.EntropyBitsPerByte = entropyBitsPerByte(counter, total)
	summary.EstimatedCompressibility = estimateCompressibility(summary.EntropyBitsPerByte)
	summary.TopBytes = topBytes(counter, total, topByteCount)
	return summary
}

func countRange(counter *bytex.Counter, start, end byte) int {
	total := 0
	for value := start; value < end; value++ {
		total += counter.Count(value)
	}
	return total
}

func entropyBitsPerByte(counter *bytex.Counter, total int) float64 {
	if total <= 0 {
		return 0
	}
	entropy := 0.0
	counter.Range(func(_ byte, count int) bool {
		probability := float64(count) / float64(total)
		entropy -= probability * math.Log2(probability)
		return true
	})
	return entropy
}

func estimateCompressibility(entropy float64) string {
	switch {
	case entropy <= 0:
		return "unknown"
	case entropy < 4.5:
		return "high"
	case entropy < 6.5:
		return "medium"
	case entropy < 7.5:
		return "low"
	default:
		return "very_low"
	}
}

func topBytes(counter *bytex.Counter, total, limit int) []ByteFrequency {
	entries := counter.MostCommon(limit)
	if len(entries) == 0 {
		return nil
	}
	out := make([]ByteFrequency, 0, len(entries))
	for _, entry := range entries {
		out = append(out, ByteFrequency{
			Value: entry.Value,
			Hex:   fmt.Sprintf("0x%02x", entry.Value),
			Count: entry.Count,
			Ratio: ratio(entry.Count, total),
		})
	}
	return out
}

func ratio(part, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(part) / float64(total)
}

func totalDeclaredAssetBytes(assets *cxmapping.Map[string, *catalog.Asset]) int64 {
	if assets == nil {
		return 0
	}
	total := int64(0)
	assets.Range(func(_ string, asset *catalog.Asset) bool {
		if asset != nil {
			total += asset.Size
		}
		return true
	})
	return total
}
