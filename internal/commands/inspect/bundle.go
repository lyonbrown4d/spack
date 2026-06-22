package inspectcmd

import (
	"strings"

	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

func inspectBundle(root string) (bundleSummary, bool, error) {
	if !spackbundle.IsBundlePath(root) {
		return bundleSummary{}, false, nil
	}
	reader, err := spackbundle.OpenReader(root)
	if err != nil {
		return bundleSummary{}, false, oops.Wrapf(err, "open bundle")
	}
	index, err := reader.Index()
	closeErr := reader.Close()
	if err != nil {
		return bundleSummary{}, false, oops.Wrapf(err, "read bundle index")
	}
	if closeErr != nil {
		return bundleSummary{}, false, oops.Wrapf(closeErr, "close bundle")
	}
	return summarizeBundleIndex(index), true, nil
}

func summarizeBundleIndex(index spackbundle.Index) bundleSummary {
	summary := bundleSummary{
		FormatVersion: index.APIVersion,
		IndexKind:     index.Kind,
		CreatedAt:     index.CreatedAt,
		FileCount:     len(index.Files),
	}
	summary.TotalBytes = lo.SumBy(index.Files, func(file spackbundle.IndexFile) int64 {
		return file.Size
	})
	summary.AssetCount = lo.CountBy(index.Files, func(file spackbundle.IndexFile) bool {
		return file.Kind == "asset" || file.Kind == ""
	})
	summary.SourceSidecarCount = lo.CountBy(index.Files, func(file spackbundle.IndexFile) bool {
		return file.Kind == "source_sidecar"
	})
	summary.CompressedFileCount = lo.CountBy(index.Files, func(file spackbundle.IndexFile) bool {
		return strings.TrimSpace(file.Encoding) != ""
	})
	return summary
}

func inspectSourceType(hasBundle bool) string {
	if hasBundle {
		return "bundle"
	}
	return "directory"
}
