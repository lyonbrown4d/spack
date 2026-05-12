package cachepolicy

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"

	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
)

// RevalidateCacheControl is the default response policy for original assets.
const RevalidateCacheControl = "public, max-age=0, must-revalidate"

// ResponsePolicy decides the cache headers emitted for resolved assets.
type ResponsePolicy interface {
	CacheControl(result *resolver.Result) string
	ExpiresAt(cacheControl string, lastModified time.Time, hasLastModified bool) (time.Time, bool)
}

// StaticResponsePolicy applies cache-header rules derived from compression config.
type StaticResponsePolicy struct {
	defaultMaxAge   time.Duration
	namespaceMaxAge *cxmapping.Map[string, time.Duration]
	immutable       immutableAssetPolicy
}

type immutableAssetPolicy struct {
	enabled bool
	maxAge  time.Duration
}

// NewResponsePolicy builds a response cache policy from compression config.
func NewResponsePolicy(cfg *config.Compression) ResponsePolicy {
	return newStaticResponsePolicy(cfg, immutableAssetPolicy{})
}

func newStaticResponsePolicy(cfg *config.Compression, immutable immutableAssetPolicy) StaticResponsePolicy {
	if cfg == nil {
		return StaticResponsePolicy{namespaceMaxAge: emptyNamespaceMaxAges(), immutable: immutable}
	}
	return StaticResponsePolicy{
		defaultMaxAge:   cfg.ParsedMaxAge(),
		namespaceMaxAge: cfg.NamespaceMaxAges(),
		immutable:       immutable,
	}
}

// NewResponsePolicyFromConfig builds a response cache policy from full runtime config.
func NewResponsePolicyFromConfig(cfg *config.Config) ResponsePolicy {
	if cfg == nil {
		return StaticResponsePolicy{namespaceMaxAge: emptyNamespaceMaxAges()}
	}

	return newStaticResponsePolicy(&cfg.Compression, immutableAssetPolicy{
		enabled: cfg.Frontend.ImmutableCache.Enable,
		maxAge:  cfg.Frontend.ImmutableCache.ParsedMaxAge(),
	})
}

func (p StaticResponsePolicy) CacheControl(result *resolver.Result) string {
	if result == nil {
		return RevalidateCacheControl
	}
	if result.Variant == nil {
		return p.assetCacheControl(result)
	}

	maxAge := p.variantMaxAge(result.Variant)
	if maxAge <= 0 {
		return RevalidateCacheControl
	}
	return fmt.Sprintf("public, max-age=%d, immutable", int(maxAge.Seconds()))
}

func (p StaticResponsePolicy) assetCacheControl(result *resolver.Result) string {
	if !p.immutable.enabled || p.immutable.maxAge <= 0 || result.Asset == nil {
		return RevalidateCacheControl
	}
	if !isFingerprintAssetPath(result.Asset.Path) {
		return RevalidateCacheControl
	}
	return fmt.Sprintf("public, max-age=%d, immutable", int(p.immutable.maxAge.Seconds()))
}

func (p StaticResponsePolicy) ExpiresAt(cacheControl string, lastModified time.Time, hasLastModified bool) (time.Time, bool) {
	_ = lastModified
	_ = hasLastModified

	maxAge, ok := cacheControlMaxAge(cacheControl)
	if !ok {
		return time.Time{}, false
	}
	return time.Now().UTC().Add(maxAge), true
}

func (p StaticResponsePolicy) variantMaxAge(variant *catalog.Variant) time.Duration {
	switch variantNamespace(variant) {
	case artifactNamespaceEncoding:
		return resolveNamespaceMaxAge(p.namespaceMaxAge, artifactNamespaceEncoding, p.defaultMaxAge)
	case artifactNamespaceImage:
		return resolveNamespaceMaxAge(p.namespaceMaxAge, artifactNamespaceImage, 0)
	default:
		return 0
	}
}

func cacheControlMaxAge(cacheControl string) (time.Duration, bool) {
	remaining := cacheControl
	for {
		part, rest, found := strings.Cut(remaining, ",")
		directive := strings.TrimSpace(part)
		key, value, hasValue := strings.Cut(directive, "=")
		if hasValue && strings.EqualFold(strings.TrimSpace(key), "max-age") {
			seconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
			if err != nil || seconds < 0 {
				return 0, false
			}
			return time.Duration(seconds) * time.Second, true
		}
		if !found {
			return 0, false
		}
		remaining = rest
	}
}

func isFingerprintAssetPath(assetPath string) bool {
	base := path.Base(strings.TrimSpace(assetPath))
	ext := path.Ext(base)
	if base == "." || ext == "" {
		return false
	}

	stem := strings.TrimSuffix(base, ext)
	if ext == ".map" {
		ext = path.Ext(stem)
		if ext == "" {
			return false
		}
		stem = strings.TrimSuffix(stem, ext)
	}
	index := strings.LastIndexAny(stem, ".-_")
	if index < 0 || index == len(stem)-1 {
		return false
	}
	return isFingerprintSegment(stem[index+1:])
}

func isFingerprintSegment(segment string) bool {
	if len(segment) < 8 {
		return false
	}
	return fingerprintSegmentPassesScan(segment)
}

func fingerprintSegmentPassesScan(segment string) bool {
	hasSignal := false
	hexOnly := true
	for _, char := range segment {
		if !unicode.IsLetter(char) && !unicode.IsDigit(char) {
			return false
		}
		if !isHexDigit(char) {
			hexOnly = false
		}
		if unicode.IsUpper(char) || unicode.IsDigit(char) {
			hasSignal = true
		}
	}
	return hasSignal || hexOnly
}

func isHexDigit(char rune) bool {
	switch {
	case char >= '0' && char <= '9':
		return true
	case char >= 'a' && char <= 'f':
		return true
	case char >= 'A' && char <= 'F':
		return true
	default:
		return false
	}
}
