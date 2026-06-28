package resolver

import (
	"github.com/lyonbrown4d/spack/pkg"
)

type acceptEntry struct {
	token string
	q     float64
}

func forEachAcceptEntry(header string, yield func(entry acceptEntry) bool) {
	pkg.ForEachAcceptEntry(header, func(entry pkg.AcceptEntry) bool {
		return yield(acceptEntry{
			token: entry.Token,
			q:     entry.Quality,
		})
	})
}

func bestAcceptQuality(current float64, hasCurrent bool, next float64) bool {
	return pkg.ShouldReplaceQuality(current, hasCurrent, next)
}

func compareAcceptQualityPriority(leftQ float64, leftPriority int, rightQ float64, rightPriority int) int {
	return pkg.CompareQualityPriority(leftQ, leftPriority, rightQ, rightPriority)
}
