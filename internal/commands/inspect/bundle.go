package inspectcmd

import (
	"fmt"
	"strings"

	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

func inspectBundle(root string) (bundleSummary, bool, error) {
	if !spackbundle.IsBundlePath(root) {
		return bundleSummary{}, false, nil
	}
	reader, err := spackbundle.OpenReader(root)
	if err != nil {
		return bundleSummary{}, false, fmt.Errorf("open bundle: %w", err)
	}
	index, err := reader.Index()
	closeErr := reader.Close()
	if err != nil {
		return bundleSummary{}, false, fmt.Errorf("read bundle index: %w", err)
	}
	if closeErr != nil {
		return bundleSummary{}, false, fmt.Errorf("close bundle: %w", closeErr)
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
	return summary
}

func inspectSourceType(hasBundle bool) string {
	if hasBundle {
		return "bundle"
	}
	return "directory"
}
