package resolver

import (
	"cmp"
	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	contentcodingspec "github.com/daiyuang/spack/internal/contentcoding/spec"
	"strings"
)

type encodingPreferences struct {
	explicit    *cxmapping.Map[string, float64]
	wildcardQ   float64
	hasWildcard bool
}

func parseAcceptEncoding(header string, supported *cxlist.List[string]) *cxlist.List[string] {
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

	supported = contentcodingspec.NormalizeNames(supported)
	if supported.IsEmpty() {
		supported = contentcodingspec.DefaultNames()
	}
	choices := cxlist.NewListWithCapacity[candidate](supported.Len())
	supported.Range(func(index int, encoding string) bool {
		q, ok := encodingQuality(prefs, encoding)
		if !ok {
			return true
		}
		choices.Add(candidate{
			encoding: encoding,
			q:        q,
			priority: index,
		})
		return true
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
