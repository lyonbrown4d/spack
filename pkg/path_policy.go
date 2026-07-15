package pkg

import "strings"

var reservedWindowsPathNames = map[string]struct{}{
	"CON":     {},
	"PRN":     {},
	"AUX":     {},
	"NUL":     {},
	"CONIN$":  {},
	"CONOUT$": {},
	"COM1":    {},
	"COM2":    {},
	"COM3":    {},
	"COM4":    {},
	"COM5":    {},
	"COM6":    {},
	"COM7":    {},
	"COM8":    {},
	"COM9":    {},
	"LPT1":    {},
	"LPT2":    {},
	"LPT3":    {},
	"LPT4":    {},
	"LPT5":    {},
	"LPT6":    {},
	"LPT7":    {},
	"LPT8":    {},
	"LPT9":    {},
}

// HasUnsafePortablePathSegment reports whether a slash-separated relative path
// contains a segment that is unsafe or ambiguous on common local filesystems.
func HasUnsafePortablePathSegment(value string) bool {
	for segment := range strings.SplitSeq(strings.ReplaceAll(value, "\\", "/"), "/") {
		if IsUnsafePortablePathSegment(segment) {
			return true
		}
	}
	return false
}

// IsUnsafePortablePathSegment reports whether segment should be rejected for
// paths that may be served on Windows or packed into cross-platform bundles.
func IsUnsafePortablePathSegment(segment string) bool {
	if segment == "" || segment == "." || segment == ".." {
		return true
	}
	if strings.Contains(segment, ":") || strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") {
		return true
	}
	name := strings.TrimRight(segment, ". ")
	base, _, _ := strings.Cut(name, ".")
	_, reserved := reservedWindowsPathNames[strings.ToUpper(base)]
	return reserved
}
