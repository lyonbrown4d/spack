package resolver

import (
	"strconv"
	"strings"
)

type acceptEntry struct {
	token string
	q     float64
}

func forEachAcceptEntry(header string, yield func(entry acceptEntry) bool) {
	remaining := header
	for {
		part, rest, found := strings.Cut(remaining, ",")
		if entry, ok := parseAcceptEntry(part); ok {
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

func parseAcceptEntry(rawPart string) (acceptEntry, bool) {
	part := strings.TrimSpace(rawPart)
	if part == "" {
		return acceptEntry{}, false
	}

	tokenRaw, paramsRaw, _ := strings.Cut(part, ";")
	token := strings.ToLower(strings.TrimSpace(tokenRaw))
	if token == "" {
		return acceptEntry{}, false
	}
	return acceptEntry{
		token: token,
		q:     parseAcceptQuality(paramsRaw),
	}, true
}

func parseAcceptQuality(params string) float64 {
	if strings.TrimSpace(params) == "" {
		return 1.0
	}

	quality := 1.0
	remaining := params
	for {
		paramRaw, rest, found := strings.Cut(remaining, ";")
		param := strings.TrimSpace(paramRaw)
		key, raw, foundEquals := strings.Cut(param, "=")
		if foundEquals && strings.EqualFold(strings.TrimSpace(key), "q") {
			quality = clampAcceptQuality(strings.TrimSpace(raw))
		}
		if !found {
			return quality
		}
		remaining = rest
	}
}

func clampAcceptQuality(raw string) float64 {
	parsed, err := strconv.ParseFloat(raw, 64)
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
