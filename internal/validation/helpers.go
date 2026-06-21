package validation

import (
	"strconv"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/normalizex"
)

func ParseFlexibleDuration(raw string) time.Duration {
	raw = normalizex.Trim(raw)
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
	return normalizex.ParsePositiveIntCSV(raw)
}
