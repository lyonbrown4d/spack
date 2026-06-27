package source

import (
	"time"

	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

const (
	SourceModeDirect  = "direct"
	SourceModeAOT     = "aot"
	SourceModeUnknown = "unknown"
)

type Stats struct {
	Mode                     string
	BundleExtractionDuration time.Duration
	BundleExtractionFiles    int
	BundleExtractionBytes    int64
}

func (s *LocalFS) Stats() Stats {
	if s == nil {
		return Stats{Mode: SourceModeUnknown}
	}
	if s.bundle == nil {
		return Stats{Mode: SourceModeDirect}
	}
	return Stats{
		Mode:                     SourceModeAOT,
		BundleExtractionDuration: s.bundleExtractionDuration,
		BundleExtractionFiles:    len(s.bundle.index.Files),
		BundleExtractionBytes:    bundleIndexBytes(s.bundle.index),
	}
}

func bundleIndexBytes(index spackbundle.Index) int64 {
	var total int64
	for fileIndex := range index.Files {
		total += index.Files[fileIndex].Size
	}
	return total
}
