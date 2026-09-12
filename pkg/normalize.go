package pkg

import (
	"cmp"
	"strconv"
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
)

type StringListOrder uint8

const (
	PreserveOrder StringListOrder = iota
	SortStrings
)

type AcceptEntry struct {
	Token   string
	Quality float64
}

func Trim(value string) string {
	return strings.TrimSpace(value)
}

func TrimLower(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func IsBlank(value string) bool {
	return strings.TrimSpace(value) == ""
}

func FirstNonBlank(values ...string) string {
	for _, value := range values {
		if !IsBlank(value) {
			return value
		}
	}
	return ""
}

func NormalizeStringList(
	values *cxlist.List[string],
	normalize func(string) string,
	order StringListOrder,
) *cxlist.List[string] {
	if values == nil || values.IsEmpty() {
		return nil
	}

	normalized := values.Stream().
		Map(func(raw string) string {
			return normalize(raw)
		}).
		Filter(func(value string) bool {
			return value != ""
		}).
		DistinctBy(func(value string) string {
			return value
		})
	if order == SortStrings {
		normalized = normalized.Sorted(strings.Compare)
	}
	return cxlist.NewList(normalized.ToSlice()...)
}

func NormalizeCSVStrings(
	raw string,
	normalize func(string) string,
	order StringListOrder,
) *cxlist.List[string] {
	if IsBlank(raw) {
		return cxlist.NewList[string]()
	}

	values := cxlist.NewList[string]()
	for part := range strings.SplitSeq(raw, ",") {
		values.Add(part)
	}
	normalized := NormalizeStringList(values, normalize, order)
	if normalized == nil {
		return cxlist.NewList[string]()
	}
	return normalized
}

func NormalizePositiveIntList(values *cxlist.List[int]) *cxlist.List[int] {
	if values == nil || values.IsEmpty() {
		return nil
	}

	normalized := values.Stream().
		Filter(func(value int) bool {
			return value > 0
		}).
		DistinctBy(func(value int) int {
			return value
		}).
		Sorted(cmp.Compare[int])
	return cxlist.NewList(normalized.ToSlice()...)
}

func ParsePositiveIntCSV(raw string) *cxlist.List[int] {
	values := cxlist.NewList[int]()
	if IsBlank(raw) {
		return values
	}

	for part := range strings.SplitSeq(raw, ",") {
		value, err := strconv.Atoi(Trim(part))
		if err == nil && value > 0 {
			values.Add(value)
		}
	}
	if values.IsEmpty() {
		return values
	}
	return NormalizePositiveIntList(values)
}

func NilIfEmpty[T any](values *cxlist.List[T]) *cxlist.List[T] {
	if values == nil || values.IsEmpty() {
		return nil
	}
	return values
}

func ForEachAcceptEntry(header string, yield func(entry AcceptEntry) bool) {
	remaining := header
	for {
		part, rest, found := strings.Cut(remaining, ",")
		if entry, ok := ParseAcceptEntry(part); ok {
			if !yield(entry) {
				return
			}
		}
		if !found {
			return
		}
		remaining = rest
	}
}

func ParseAcceptEntry(rawPart string) (AcceptEntry, bool) {
	part := Trim(rawPart)
	if part == "" {
		return AcceptEntry{}, false
	}

	tokenRaw, paramsRaw, _ := strings.Cut(part, ";")
	token := TrimLower(tokenRaw)
	if token == "" {
		return AcceptEntry{}, false
	}
	return AcceptEntry{
		Token:   token,
		Quality: ParseAcceptQuality(paramsRaw),
	}, true
}

func ParseAcceptQuality(params string) float64 {
	if IsBlank(params) {
		return 1.0
	}

	quality := 1.0
	remaining := params
	for {
		paramRaw, rest, found := strings.Cut(remaining, ";")
		param := Trim(paramRaw)
		key, raw, foundEquals := strings.Cut(param, "=")
		if foundEquals && strings.EqualFold(Trim(key), "q") {
			quality = ClampAcceptQuality(raw)
		}
		if !found {
			return quality
		}
		remaining = rest
	}
}

func ClampAcceptQuality(raw string) float64 {
	parsed, err := strconv.ParseFloat(Trim(raw), 64)
	if err != nil {
		return 1.0
	}
	if parsed < 0 {
		return 0
	}
	if parsed > 1 {
		return 1
	}
	return parsed
}

func ShouldReplaceQuality(current float64, hasCurrent bool, next float64) bool {
	return !hasCurrent || next > current
}

func CompareQualityPriority(leftQ float64, leftPriority int, rightQ float64, rightPriority int) int {
	if leftQ == rightQ {
		return cmp.Compare(leftPriority, rightPriority)
	}
	if leftQ > rightQ {
		return -1
	}
	return 1
}
