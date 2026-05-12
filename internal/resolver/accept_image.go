package resolver

import (
	"cmp"
	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/media"
	"strings"
)

type imagePreferences struct {
	explicit         *cxmapping.Map[string, float64]
	wildcardImageQ   float64
	hasWildcardImage bool
	wildcardAnyQ     float64
	hasWildcardAny   bool
}

type imagePreferenceMatch int

const (
	imagePreferenceNone imagePreferenceMatch = iota
	imagePreferenceAnyWildcard
	imagePreferenceImageWildcard
	imagePreferenceExplicit
)

func parseAcceptImageFormats(header, sourceFormat string, supported *cxlist.List[string]) *cxlist.List[string] {
	if strings.TrimSpace(header) == "" {
		return nil
	}

	return buildImageCandidates(collectImagePreferences(header), sourceFormat, supported)
}

func collectImagePreferences(header string) imagePreferences {
	prefs := imagePreferences{
		explicit: cxmapping.NewMapWithCapacity[string, float64](4),
	}
	forEachAcceptEntry(header, func(entry acceptEntry) bool {
		applyImagePreference(&prefs, entry)
		return true
	})
	return prefs
}

func applyImagePreference(prefs *imagePreferences, entry acceptEntry) {
	switch entry.token {
	case "image/*":
		prefs.hasWildcardImage = true
		prefs.wildcardImageQ = entry.q
	case "*/*":
		prefs.hasWildcardAny = true
		prefs.wildcardAnyQ = entry.q
	default:
		if descriptor, ok := media.LookupImageDescriptorByAcceptToken(entry.token); ok {
			setMaxQuality(prefs.explicit, descriptor.Name, entry.q)
		}
	}
}

// setMaxQuality updates key to the maximum of its current value and q (using -1 when key is absent).
// Used for Accept-Encoding tokens and image format names.
func setMaxQuality(values *cxmapping.Map[string, float64], key string, q float64) {
	if q <= values.GetOrDefault(key, -1) {
		return
	}
	values.Set(key, q)
}

func buildImageCandidates(prefs imagePreferences, sourceFormat string, supported *cxlist.List[string]) *cxlist.List[string] {
	type candidate struct {
		format   string
		q        float64
		match    imagePreferenceMatch
		priority int
	}

	supported = imageFormatCandidates(supported, sourceFormat)
	candidates := cxlist.NewListWithCapacity[candidate](supported.Len())
	supported.Range(func(index int, format string) bool {
		q, match := imageQualityForFormat(prefs, format)
		if q <= 0 || match == imagePreferenceNone {
			return true
		}
		candidates.Add(candidate{
			format:   format,
			q:        q,
			match:    match,
			priority: imagePriority(index, format, sourceFormat),
		})
		return true
	})

	candidates.Sort(func(left, right candidate) int {
		if left.match != right.match {
			return cmp.Compare(int(right.match), int(left.match))
		}
		if left.q == right.q {
			return cmp.Compare(left.priority, right.priority)
		}
		if left.q > right.q {
			return -1
		}
		return 1
	})

	if candidates.IsEmpty() {
		return nil
	}
	return cxlist.MapList[candidate, string](candidates, func(_ int, candidate candidate) string {
		return candidate.format
	})
}

func imageFormatCandidates(supported *cxlist.List[string], sourceFormat string) *cxlist.List[string] {
	candidates := cxlist.NewList[string]()
	if supported != nil && !supported.IsEmpty() {
		candidates.Add(supported.Values()...)
	}
	if sourceFormat != "" {
		candidates.Add(sourceFormat)
	}
	if candidates.IsEmpty() {
		candidates.Add(media.SupportedImageFormats().Values()...)
	}
	return media.NormalizeImageFormats(candidates)
}

func imageQualityForFormat(prefs imagePreferences, format string) (float64, imagePreferenceMatch) {
	if q, ok := prefs.explicit.Get(format); ok {
		return q, imagePreferenceExplicit
	}
	if prefs.hasWildcardImage {
		return prefs.wildcardImageQ, imagePreferenceImageWildcard
	}
	if prefs.hasWildcardAny {
		return prefs.wildcardAnyQ, imagePreferenceAnyWildcard
	}
	return 0, imagePreferenceNone
}

func imagePriority(index int, format, sourceFormat string) int {
	if format == sourceFormat {
		return -1
	}
	return index
}
