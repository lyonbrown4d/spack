package resolver

import (
	"cmp"
	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	contentcodingspec "github.com/lyonbrown4d/spack/internal/contentcoding/spec"
	"strings"
)

var defaultSupportedEncodings = contentcodingspec.DefaultNames()

type encodingPreferences struct {
	explicit    *cxmapping.Map[string, float64]
	wildcardQ   float64
	hasWildcard bool
}

func parseAcceptEncoding(header string, supported *cxlist.List[string]) *cxlist.List[string] {
	return ParseAcceptEncodingNormalized(header, supported)
}

func ParseAcceptEncoding(header string, supported *cxlist.List[string]) *cxlist.List[string] {
	if strings.TrimSpace(header) == "" {
		return nil
	}

	return buildEncodingCandidates(collectEncodingPreferences(header), contentcodingspec.NormalizeNames(supported))
}

func ParseAcceptEncodingNormalized(header string, supported *cxlist.List[string]) *cxlist.List[string] {
	if strings.TrimSpace(header) == "" {
		return nil
	}

	return buildEncodingCandidates(collectEncodingPreferences(header), supported)
}

func collectEncodingPreferences(header string) encodingPreferences {
	prefs := encodingPreferences{
		explicit: cxmapping.NewMapWithCapacity[string, float64](4),
	}
	forEachAcceptEntry(header, func(entry acceptEntry) bool {
		if entry.token == "*" {
			prefs.hasWildcard = true
			prefs.wildcardQ = entry.q
			return true
		}
		setMaxQuality(prefs.explicit, entry.token, entry.q)
		return true
	})
	return prefs
}

func buildEncodingCandidates(prefs encodingPreferences, supported *cxlist.List[string]) *cxlist.List[string] {
	type candidate struct {
		encoding string
		q        float64
		priority int
	}

	if supported.IsEmpty() {
		supported = defaultSupportedEncodings
	}
	choices := cxlist.FilterMapList[string, candidate](supported, func(index int, encoding string) (candidate, bool) {
		q, ok := encodingQuality(prefs, encoding)
		if !ok {
			return candidate{}, false
		}
		return candidate{
			encoding: encoding,
			q:        q,
			priority: index,
		}, true
	})

	choices.Sort(func(left, right candidate) int {
		if left.q == right.q {
			return cmp.Compare(left.priority, right.priority)
		}
		if left.q > right.q {
			return -1
		}
		return 1
	})

	if choices.IsEmpty() {
		return nil
	}
	return cxlist.MapList[candidate, string](choices, func(_ int, choice candidate) string {
		return choice.encoding
	})
}

func encodingQuality(prefs encodingPreferences, encoding string) (float64, bool) {
	if q, ok := prefs.explicit.Get(encoding); ok {
		return q, q > 0
	}
	if !prefs.hasWildcard || prefs.wildcardQ <= 0 {
		return 0, false
	}
	return prefs.wildcardQ, true
}
