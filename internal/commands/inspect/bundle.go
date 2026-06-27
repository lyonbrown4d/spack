package inspectcmd

import (
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/samber/mo"
)

func inspectBundle(resolved source.Resolved) mo.Option[bundleSummary] {
	if resolved.Type != source.TypeBundle || resolved.Bundle == nil {
		return mo.None[bundleSummary]()
	}
	return mo.Some(summarizeBundleMetadata(resolved.Bundle))
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
