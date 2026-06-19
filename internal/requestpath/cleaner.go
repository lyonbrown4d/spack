// Package requestpath normalizes incoming HTTP request paths for asset resolution.
package requestpath

import (
	"net/url"
	"path"
	"strings"

	"github.com/samber/lo"
)

type Cleaned struct {
	Value               string
	AllowsEntryFallback bool
}

func Clean(raw string) Cleaned {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Cleaned{AllowsEntryFallback: true}
	}

	if strings.IndexByte(trimmed, '%') >= 0 {
		trimmed = decode(trimmed)
	}
	if cleaned, ok := fastClean(trimmed); ok {
		return cleaned
	}

	clean := path.Clean("/" + strings.TrimPrefix(trimmed, "/"))
	if clean == "/" || clean == "." {
		return Cleaned{AllowsEntryFallback: true}
	}

	value := strings.TrimPrefix(clean, "/")
	return Cleaned{
		Value:               value,
		AllowsEntryFallback: strings.HasSuffix(trimmed, "/") || !hasExtension(value),
	}
}

func CleanMounted(requestPath, mountPath string) Cleaned {
	return Clean(TrimMount(requestPath, mountPath))
}

func MountPatterns(mountPath string) []string {
	mount := cleanMountPath(mountPath)
	if mount == "/" {
		return []string{"/*"}
	}
	return []string{mount, mount + "/*"}
}

func TrimMount(requestPath, mountPath string) string {
	mount := cleanMountPath(mountPath)
	if mount == "/" {
		return strings.TrimPrefix(requestPath, "/")
	}

	if requestPath == mount {
		return ""
	}
	prefix := mount + "/"
	if strings.HasPrefix(requestPath, prefix) {
		return requestPath[len(prefix):]
	}
	return strings.TrimPrefix(requestPath, "/")
}

func cleanMountPath(mountPath string) string {
	mount := strings.TrimSpace(mountPath)
	if mount == "" || mount == "/" {
		return "/"
	}
	return "/" + strings.Trim(strings.TrimRight(mount, "/"), "/")
}

func decode(raw string) string {
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return raw
	}
	return decoded
}

func fastClean(raw string) (Cleaned, bool) {
	value, ok := canonicalValue(raw)
	if !ok {
		return Cleaned{}, false
	}
	return Cleaned{
		Value:               value,
		AllowsEntryFallback: !hasExtension(value),
	}, true
}

func canonicalValue(raw string) (string, bool) {
	value, ok := stripSingleLeadingSlash(raw)
	if !ok || rejectsFastPathValue(value) {
		return "", false
	}
	if !hasCanonicalSegments(value) {
		return "", false
	}
	return value, true
}

func stripSingleLeadingSlash(raw string) (string, bool) {
	if raw == "" || raw == "/" || raw == "." {
		return "", false
	}
	if raw[0] != '/' {
		return raw, true
	}
	if len(raw) == 1 || raw[1] == '/' {
		return "", false
	}
	return raw[1:], true
}

func rejectsFastPathValue(value string) bool {
	return value == "" || strings.HasSuffix(value, "/") || strings.ContainsRune(value, '\\')
}

func hasCanonicalSegments(value string) bool {
	return lo.EveryBy(strings.Split(value, "/"), isCanonicalSegment)
}

func isCanonicalSegment(segment string) bool {
	return segment != "" && segment != "." && segment != ".."
}

func hasExtension(value string) bool {
	slash := strings.LastIndexByte(value, '/')
	dot := strings.LastIndexByte(value, '.')
	return dot > slash && dot != -1 && dot > 0
}
