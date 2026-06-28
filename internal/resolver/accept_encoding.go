package resolver

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	contentcodingspec "github.com/lyonbrown4d/spack/internal/contentcoding/spec"
	"github.com/lyonbrown4d/spack/pkg"
	"slices"
)

var defaultSupportedEncodings = contentcodingspec.DefaultNames()

type encodingMask uint8

const (
	encodingMaskBr encodingMask = 1 << iota
	encodingMaskZstd
	encodingMaskGzip
	encodingMaskIdentity
)

type encodingPreferences struct {
	brQ       float64
	zstdQ     float64
	gzipQ     float64
	identityQ float64
	known     encodingMask
	extra     map[string]float64

	wildcardQ   float64
	hasWildcard bool
}

type encodingCandidate struct {
	encoding string
	q        float64
	priority int
}

func parseAcceptEncoding(header string, supported *cxlist.List[string]) *cxlist.List[string] {
	return ParseAcceptEncodingNormalized(header, supported)
}

func ParseAcceptEncoding(header string, supported *cxlist.List[string]) *cxlist.List[string] {
	if pkg.IsBlank(header) {
		return nil
	}

	supported = encodingSupportedCandidates(contentcodingspec.NormalizeNames(supported))
	return buildEncodingCandidates(collectEncodingPreferences(header, supported), supported)
}

func ParseAcceptEncodingNormalized(header string, supported *cxlist.List[string]) *cxlist.List[string] {
	if pkg.IsBlank(header) {
		return nil
	}

	supported = encodingSupportedCandidates(supported)
	return buildEncodingCandidates(collectEncodingPreferences(header, supported), supported)
}

func collectEncodingPreferences(header string, supported *cxlist.List[string]) encodingPreferences {
	var prefs encodingPreferences
	forEachAcceptEntry(header, func(entry acceptEntry) bool {
		applyEncodingPreference(&prefs, supported, entry)
		return true
	})
	return prefs
}

func applyEncodingPreference(prefs *encodingPreferences, supported *cxlist.List[string], entry acceptEntry) {
	if entry.token == "*" {
		prefs.hasWildcard = true
		prefs.wildcardQ = entry.q
		return
	}
	if mask, ok := knownEncodingMask(entry.token); ok {
		prefs.setKnown(mask, entry.q)
		return
	}
	if supportsEncodingToken(supported, entry.token) {
		applyExtraEncodingPreference(prefs, entry)
	}
}

func knownEncodingMask(token string) (encodingMask, bool) {
	switch token {
	case "br":
		return encodingMaskBr, true
	case "zstd":
		return encodingMaskZstd, true
	case "gzip":
		return encodingMaskGzip, true
	case "identity":
		return encodingMaskIdentity, true
	}
	return 0, false
}

func (prefs *encodingPreferences) setKnown(mask encodingMask, q float64) {
	current := prefs.knownQuality(mask)
	if !bestAcceptQuality(current, prefs.known.has(mask), q) {
		return
	}
	prefs.known |= mask
	prefs.setKnownQuality(mask, q)
}

func (mask encodingMask) has(value encodingMask) bool {
	return mask&value != 0
}

func (prefs encodingPreferences) knownQuality(mask encodingMask) float64 {
	switch mask {
	case encodingMaskBr:
		return prefs.brQ
	case encodingMaskZstd:
		return prefs.zstdQ
	case encodingMaskGzip:
		return prefs.gzipQ
	case encodingMaskIdentity:
		return prefs.identityQ
	default:
		return 0
	}
}

func (prefs *encodingPreferences) setKnownQuality(mask encodingMask, q float64) {
	switch mask {
	case encodingMaskBr:
		prefs.brQ = q
	case encodingMaskZstd:
		prefs.zstdQ = q
	case encodingMaskGzip:
		prefs.gzipQ = q
	case encodingMaskIdentity:
		prefs.identityQ = q
	}
}

func applyExtraEncodingPreference(prefs *encodingPreferences, entry acceptEntry) {
	if prefs.extra == nil {
		prefs.extra = make(map[string]float64, 1)
	}
	current, ok := prefs.extra[entry.token]
	if bestAcceptQuality(current, ok, entry.q) {
		prefs.extra[entry.token] = entry.q
	}
}

func buildEncodingCandidates(prefs encodingPreferences, supported *cxlist.List[string]) *cxlist.List[string] {
	var stack [4]encodingCandidate
	choices := stack[:0]
	supported.Range(func(index int, encoding string) bool {
		q, ok := encodingQuality(prefs, encoding)
		if !ok {
			return true
		}
		choices = append(choices, encodingCandidate{
			encoding: encoding,
			q:        q,
			priority: index,
		})
		return true
	})

	if len(choices) == 0 {
		return nil
	}
	slices.SortFunc(choices, compareEncodingCandidates)

	encodings := cxlist.NewListWithCapacity[string](len(choices))
	for _, choice := range choices {
		encodings.Add(choice.encoding)
	}
	return encodings
}

func encodingQuality(prefs encodingPreferences, encoding string) (float64, bool) {
	if q, ok := knownEncodingQuality(prefs, encoding); ok {
		return q, q > 0
	}
	if q, ok := prefs.extra[encoding]; ok {
		return q, q > 0
	}
	return wildcardEncodingQuality(prefs)
}

func knownEncodingQuality(prefs encodingPreferences, encoding string) (float64, bool) {
	mask, ok := knownEncodingMask(encoding)
	if !ok || !prefs.known.has(mask) {
		return 0, false
	}
	return prefs.knownQuality(mask), true
}

func wildcardEncodingQuality(prefs encodingPreferences) (float64, bool) {
	if !prefs.hasWildcard || prefs.wildcardQ <= 0 {
		return 0, false
	}
	return prefs.wildcardQ, true
}

func encodingSupportedCandidates(supported *cxlist.List[string]) *cxlist.List[string] {
	supported = pkg.NilIfEmpty(
		pkg.NormalizeStringList(supported, pkg.TrimLower, pkg.PreserveOrder),
	)
	if supported == nil {
		return defaultSupportedEncodings
	}
	return supported
}

func supportsEncodingToken(supported *cxlist.List[string], token string) bool {
	supportedEncoding := false
	supported.Range(func(_ int, encoding string) bool {
		if encoding == token {
			supportedEncoding = true
			return false
		}
		return true
	})
	return supportedEncoding
}

func compareEncodingCandidates(left, right encodingCandidate) int {
	return compareAcceptQualityPriority(left.q, left.priority, right.q, right.priority)
}
