package inspectcmd

import "github.com/lyonbrown4d/spack/internal/source"

func inspectBundle(resolved source.Resolved) (bundleSummary, bool) {
	if resolved.Type != source.TypeBundle || resolved.Bundle == nil {
		return bundleSummary{}, false
	}
	return summarizeBundleMetadata(resolved.Bundle), true
}

func summarizeBundleMetadata(bundle *source.BundleMetadata) bundleSummary {
	return bundleSummary{
		FormatVersion:       bundle.FormatVersion,
		IndexKind:           bundle.IndexKind,
		CreatedAt:           bundle.CreatedAt,
		FileCount:           bundle.FileCount,
		AssetCount:          bundle.AssetCount,
		SourceSidecarCount:  bundle.SourceSidecarCount,
		CompressedFileCount: bundle.CompressedFileCount,
		TotalBytes:          bundle.TotalBytes,
	}
}
