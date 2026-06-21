package pipeline

import (
	"github.com/lyonbrown4d/spack/internal/normalizex"
)

func requestKey(request Request) string {
	assetPath := normalizex.Trim(request.AssetPath)
	encodings := normalizeRequestStrings(request.PreferredEncodings)
	formats := normalizeRequestStrings(request.PreferredFormats)
	widths := normalizeRequestInts(request.PreferredWidths)
	return buildRequestKey(assetPath, encodings, formats, widths)
}
