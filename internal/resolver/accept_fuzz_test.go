package resolver_test

import (
	"testing"

	"github.com/lyonbrown4d/spack/internal/resolver"
)

func FuzzParseAcceptEncoding(f *testing.F) {
	for _, seed := range []string{
		"",
		"br,gzip",
		"gzip;q=0.8, zstd;q=0.9, br;q=1.0",
		"*;q=0.5, gzip;q=0",
		"identity;q=1, br;q=0.7",
		"gzip;level=9;q=0.4 ;x=1, br;Q=0.9;foo=bar",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, header string) {
		if len(header) > 4096 {
			t.Skip("header seed is larger than the parser budget")
		}
		assertUniqueSupportedValues(t, resolver.ParseAcceptEncodingForTest(header).Values(), map[string]struct{}{
			"br":   {},
			"zstd": {},
			"gzip": {},
		})
	})
}

func FuzzParseAcceptImageFormats(f *testing.F) {
	for _, seed := range [][2]string{
		{"", "jpeg"},
		{"image/png;q=1,image/jpeg;q=0.6,*/*;q=0.1", "jpeg"},
		{"image/jpeg;q=0.7,image/*;q=0.9", "png"},
		{"image/webp,image/avif,image/png;q=0.5", "png"},
		{"*/*;q=1", ""},
	} {
		f.Add(seed[0], seed[1])
	}

	f.Fuzz(func(t *testing.T, header, sourceFormat string) {
		if len(header) > 4096 || len(sourceFormat) > 64 {
			t.Skip("header seed is larger than the parser budget")
		}
		assertUniqueSupportedValues(t, resolver.ParseAcceptImageFormatsForTest(header, sourceFormat).Values(), map[string]struct{}{
			"jpeg": {},
			"png":  {},
			"webp": {},
			"avif": {},
		})
	})
}

func assertUniqueSupportedValues(t *testing.T, values []string, supported map[string]struct{}) {
	t.Helper()

	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := supported[value]; !ok {
			t.Fatalf("parser returned unsupported value %q from %#v", value, values)
		}
		if _, ok := seen[value]; ok {
			t.Fatalf("parser returned duplicate value %q from %#v", value, values)
		}
		seen[value] = struct{}{}
	}
}
