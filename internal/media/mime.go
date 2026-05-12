package media

import (
	"path/filepath"
	"strings"

	cxset "github.com/arcgolabs/collectionx/set"
	"github.com/lyonbrown4d/spack/internal/constant"
)

var textLikeMediaTypes = cxset.NewOrderedSet[string](
	string(constant.ApplicationJavascript),
	string(constant.XJavascript),
	string(constant.JSON),
	string(constant.ManifestJSON),
	string(constant.XML),
	string(constant.XHTML),
	string(constant.Svg),
)

var compressibleNonTextMediaTypes = cxset.NewOrderedSet[string](
	string(constant.Wasm),
)

var textLikeFileExtensions = cxset.NewOrderedSet[string](
	".html",
	".css",
	".js",
	".mjs",
	".json",
	".xml",
	".txt",
	".svg",
	".webmanifest",
)

func NormalizeMediaType(mediaType string) string {
	return strings.ToLower(strings.TrimSpace(mediaType))
}

func IsTextLikeMediaType(mediaType string) bool {
	normalized := NormalizeMediaType(mediaType)
	switch {
	case strings.HasPrefix(normalized, "text/"):
		return true
	case textLikeMediaTypes.Contains(normalized):
		return true
	default:
		return !IsImageMediaType(normalized) && strings.Contains(normalized, "json")
	}
}

func IsTextLikeFileExtension(pathOrExt string) bool {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(pathOrExt)))
	return textLikeFileExtensions.Contains(ext)
}

func IsNonCompressibleMediaType(mediaType string) bool {
	normalized := NormalizeMediaType(mediaType)
	if strings.HasPrefix(normalized, "image/") && normalized != string(constant.Svg) {
		return true
	}
	if strings.HasPrefix(normalized, "audio/") || strings.HasPrefix(normalized, "video/") {
		return true
	}
	return strings.Contains(normalized, "zip") || strings.Contains(normalized, "gzip")
}

func IsCompressibleMediaType(mediaType string) bool {
	normalized := NormalizeMediaType(mediaType)
	switch {
	case normalized == "":
		return false
	case IsNonCompressibleMediaType(normalized):
		return false
	case IsTextLikeMediaType(normalized):
		return true
	default:
		return compressibleNonTextMediaTypes.Contains(normalized)
	}
}
