package resolver

import (
	"cmp"
	"slices"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/media"
	"github.com/lyonbrown4d/spack/pkg"
)

var defaultImageFormatSupport = newImageFormatSupport(media.SupportedImageFormats())

type imageFormatMask uint8

const (
	imageFormatMaskJPEG imageFormatMask = 1 << iota
	imageFormatMaskPNG
	imageFormatMaskWebP
	imageFormatMaskAVIF
)

type imageFormatSupport struct {
	values *cxlist.List[string]
	known  imageFormatMask
}

type imagePreferences struct {
	jpegQ            float64
	pngQ             float64
	webpQ            float64
	avifQ            float64
	explicit         imageFormatMask
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

func parseAcceptImageFormats(header, sourceFormat string, supported imageFormatSupport) *cxlist.List[string] {
	return parseAcceptImageFormatsWithSupport(header, sourceFormat, supported)
}

func ParseAcceptImageFormats(header, sourceFormat string, supported *cxlist.List[string]) *cxlist.List[string] {
	return parseAcceptImageFormatsWithSupport(header, sourceFormat, newImageFormatSupport(supported))
}

func parseAcceptImageFormatsWithSupport(header, sourceFormat string, supported imageFormatSupport) *cxlist.List[string] {
	if pkg.IsBlank(header) {
		return nil
	}

	return buildImageCandidates(collectImagePreferences(header), media.NormalizeImageFormat(sourceFormat), supported)
}

func newImageFormatSupport(supported *cxlist.List[string]) imageFormatSupport {
	supported = media.NormalizeImageFormats(supported)
	if supported == nil || supported.IsEmpty() {
		supported = media.SupportedImageFormats()
	}

	var known imageFormatMask
	supported.Range(func(_ int, format string) bool {
		if mask, ok := imageFormatMaskForName(format); ok {
			known |= mask
		}
		return true
	})
	return imageFormatSupport{values: supported, known: known}
}

func (support imageFormatSupport) withSourceFormat(sourceFormat string) imageFormatSupport {
	if support.values == nil {
		support = defaultImageFormatSupport
	}
	if sourceFormat == "" {
		return support
	}
	mask, ok := imageFormatMaskForName(sourceFormat)
	if ok && support.known.has(mask) {
		return support
	}

	values := cxlist.NewListWithCapacity[string](support.values.Len() + 1)
	values.Merge(support.values)
	values.Add(sourceFormat)
	return newImageFormatSupport(values)
}

func collectImagePreferences(header string) imagePreferences {
	var prefs imagePreferences
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
			if mask, ok := imageFormatMaskForName(descriptor.Name); ok {
				prefs.setExplicit(mask, entry.q)
			}
		}
	}
}

func imageFormatMaskForName(format string) (imageFormatMask, bool) {
	switch format {
	case "jpeg":
		return imageFormatMaskJPEG, true
	case "png":
		return imageFormatMaskPNG, true
	case "webp":
		return imageFormatMaskWebP, true
	case "avif":
		return imageFormatMaskAVIF, true
	default:
		return 0, false
	}
}

func (prefs *imagePreferences) setExplicit(mask imageFormatMask, q float64) {
	if !bestAcceptQuality(prefs.quality(mask), prefs.explicit.has(mask), q) {
		return
	}
	prefs.explicit |= mask
	prefs.setQuality(mask, q)
}

func (mask imageFormatMask) has(value imageFormatMask) bool {
	return mask&value != 0
}

func (prefs imagePreferences) quality(mask imageFormatMask) float64 {
	switch mask {
	case imageFormatMaskJPEG:
		return prefs.jpegQ
	case imageFormatMaskPNG:
		return prefs.pngQ
	case imageFormatMaskWebP:
		return prefs.webpQ
	case imageFormatMaskAVIF:
		return prefs.avifQ
	default:
		return 0
	}
}

func (prefs *imagePreferences) setQuality(mask imageFormatMask, q float64) {
	switch mask {
	case imageFormatMaskJPEG:
		prefs.jpegQ = q
	case imageFormatMaskPNG:
		prefs.pngQ = q
	case imageFormatMaskWebP:
		prefs.webpQ = q
	case imageFormatMaskAVIF:
		prefs.avifQ = q
	}
}

func buildImageCandidates(prefs imagePreferences, sourceFormat string, supported imageFormatSupport) *cxlist.List[string] {
	supported = supported.withSourceFormat(sourceFormat)
	var stack [4]imageCandidate
	candidates := stack[:0]
	supported.values.Range(func(index int, format string) bool {
		q, match := imageQualityForFormat(prefs, format)
		if q <= 0 || match == imagePreferenceNone {
			return true
		}
		candidates = append(candidates, imageCandidate{
			format:   format,
			q:        q,
			match:    match,
			priority: imagePriority(index, format, sourceFormat),
		})
		return true
	})

	if len(candidates) == 0 {
		return nil
	}
	slices.SortFunc(candidates, compareImageCandidates)

	formats := cxlist.NewListWithCapacity[string](len(candidates))
	for _, candidate := range candidates {
		formats.Add(candidate.format)
	}
	return formats
}

type imageCandidate struct {
	format   string
	q        float64
	match    imagePreferenceMatch
	priority int
}

func compareImageCandidates(left, right imageCandidate) int {
	if left.match != right.match {
		return cmp.Compare(int(right.match), int(left.match))
	}
	return compareAcceptQualityPriority(left.q, left.priority, right.q, right.priority)
}

func imageQualityForFormat(prefs imagePreferences, format string) (float64, imagePreferenceMatch) {
	if mask, ok := imageFormatMaskForName(format); ok && prefs.explicit.has(mask) {
		return prefs.quality(mask), imagePreferenceExplicit
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
