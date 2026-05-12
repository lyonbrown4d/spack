package validation

import (
	"cmp"
	"strconv"
	"strings"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxset "github.com/arcgolabs/collectionx/set"
)

func ParseFlexibleDuration(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err == nil {
		if d > 0 {
			return d
		}
		return 0
	}

	seconds, secErr := strconv.ParseInt(raw, 10, 64)
	if secErr != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func ParseWidths(raw string) *cxlist.List[int] {
	if strings.TrimSpace(raw) == "" {
		return cxlist.NewList[int]()
	}

	widths := cxlist.NewList[int]()
	for _, part := range strings.Split(raw, ",") {
		width, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || width <= 0 {
			continue
		}
		widths.Add(width)
	}
	if widths.IsEmpty() {
		return widths
	}

	widths.Sort(cmp.Compare[int])
	unique := cxset.NewOrderedSetWithCapacity[int](widths.Len())
	widths.Range(func(_ int, width int) bool {
		unique.Add(width)
		return true
	})
	return cxlist.NewList[int](unique.Values()...)
}
