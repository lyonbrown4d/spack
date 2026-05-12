package cachepolicy

import (
	"strings"
	"time"

	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/catalog"
)

const (
	artifactNamespaceEncoding = "encoding"
	artifactNamespaceImage    = "image"
)

func emptyNamespaceMaxAges() *cxmapping.Map[string, time.Duration] {
	return cxmapping.NewMap[string, time.Duration]()
}

func resolveNamespaceMaxAge(
	namespaceMaxAge *cxmapping.Map[string, time.Duration],
	namespace string,
	fallback time.Duration,
) time.Duration {
	if maxAge, ok := namespaceMaxAge.Get(namespace); ok && maxAge > 0 {
		return maxAge
	}
	return fallback
}

func variantNamespace(variant *catalog.Variant) string {
	switch {
	case variant == nil:
		return ""
	case strings.TrimSpace(variant.Encoding) != "":
		return artifactNamespaceEncoding
	case variant.Width > 0 || strings.TrimSpace(variant.Format) != "":
		return artifactNamespaceImage
	default:
		return ""
	}
}
