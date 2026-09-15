package cachepolicy_test

import (
	"testing"
	"time"

	"github.com/lyonbrown4d/spack/internal/cachepolicy"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
)

func TestResponsePolicyUsesDefaultMaxAgeForEncodingVariants(t *testing.T) {
	policy := cachepolicy.NewResponsePolicy(&config.Compression{
		MaxAge: "168h",
	})

	cacheControl := policy.CacheControl(&resolver.Result{
		Variant: &catalog.Variant{Encoding: "br"},
	})
	if cacheControl != "public, max-age=604800, immutable" {
		t.Fatalf("unexpected cache-control %q", cacheControl)
	}
}

func TestResponsePolicyDoesNotFallbackImageVariantsToDefaultMaxAge(t *testing.T) {
	policy := cachepolicy.NewResponsePolicy(&config.Compression{
		MaxAge: "168h",
	})

	cacheControl := policy.CacheControl(&resolver.Result{
		Variant: &catalog.Variant{Width: 640},
	})
	if cacheControl != cachepolicy.RevalidateCacheControl {
		t.Fatalf("expected revalidate cache-control, got %q", cacheControl)
	}
}

func TestResponsePolicyUsesImageNamespaceMaxAgeForImageVariants(t *testing.T) {
	policy := cachepolicy.NewResponsePolicy(&config.Compression{
		MaxAge:      "168h",
		ImageMaxAge: "336h",
	})

	cacheControl := policy.CacheControl(&resolver.Result{
		Variant: &catalog.Variant{Width: 640},
	})
	if cacheControl != "public, max-age=1209600, immutable" {
		t.Fatalf("unexpected cache-control %q", cacheControl)
	}
}

func TestResponsePolicyUsesImmutableCacheForFingerprintedAssets(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	cfg.Frontend.ImmutableCache.MaxAge = "24h"
	policy := cachepolicy.NewResponsePolicyFromConfig(&cfg)

	cacheControl := policy.CacheControl(&resolver.Result{
		Asset: &catalog.Asset{Path: "assets/app-deadbeef.js"},
	})
	if cacheControl != "public, max-age=86400, immutable" {
		t.Fatalf("unexpected cache-control %q", cacheControl)
	}
}

func TestResponsePolicyKeepsEntryAssetsRevalidated(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	policy := cachepolicy.NewResponsePolicyFromConfig(&cfg)

	cacheControl := policy.CacheControl(&resolver.Result{
		Asset: &catalog.Asset{Path: "index.html"},
	})
	if cacheControl != cachepolicy.HTMLRevalidateCacheControl {
		t.Fatalf("expected revalidate cache-control, got %q", cacheControl)
	}
}

func TestResponsePolicyUsesImmutableCacheForFingerprintedSourceMaps(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	cfg.Frontend.ImmutableCache.MaxAge = "24h"
	policy := cachepolicy.NewResponsePolicyFromConfig(&cfg)

	cacheControl := policy.CacheControl(&resolver.Result{
		Asset: &catalog.Asset{Path: "assets/app-deadbeef.js.map"},
	})
	if cacheControl != "public, max-age=86400, immutable" {
		t.Fatalf("unexpected cache-control %q", cacheControl)
	}
}

func TestResponsePolicyExpiresAtUsesMaxAge(t *testing.T) {
	policy := cachepolicy.NewResponsePolicy(&config.Compression{})

	before := time.Now().UTC()
	expiresAt, ok := policy.ExpiresAt("public, max-age=60, immutable", time.Time{}, false)
	after := time.Now().UTC()
	if !ok {
		t.Fatal("expected expires-at to be derived from max-age")
	}
	if expiresAt.Before(before.Add(55*time.Second)) || expiresAt.After(after.Add(65*time.Second)) {
		t.Fatalf("expected expires-at to be about 60s from now, got %s", expiresAt)
	}
}

func TestResponsePolicyExpiresAtRejectsInvalidMaxAge(t *testing.T) {
	policy := cachepolicy.NewResponsePolicy(&config.Compression{})

	if _, ok := policy.ExpiresAt("public, immutable", time.Time{}, false); ok {
		t.Fatal("expected missing max-age to return false")
	}
	if _, ok := policy.ExpiresAt("public, max-age=oops", time.Time{}, false); ok {
		t.Fatal("expected invalid max-age to return false")
	}
}

func TestResponsePolicyRevalidatesHTMLAndFallbackAcrossEncodings(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	policy := cachepolicy.NewResponsePolicyFromConfig(&cfg)
	for _, encoding := range []string{"", "br", "gzip", "zstd"} {
		t.Run(encoding, func(t *testing.T) {
			result := &resolver.Result{
				Asset:     &catalog.Asset{Path: "index.html", MediaType: "text/html; charset=utf-8"},
				MediaType: "text/html; charset=utf-8",
			}
			if encoding != "" {
				result.Variant = &catalog.Variant{Encoding: encoding}
			}
			if got := policy.CacheControl(result); got != cachepolicy.HTMLRevalidateCacheControl {
				t.Fatalf("HTML encoding %q: got %q", encoding, got)
			}
			result.FallbackUsed = true
			if got := policy.CacheControl(result); got != cachepolicy.HTMLRevalidateCacheControl {
				t.Fatalf("fallback encoding %q: got %q", encoding, got)
			}
		})
	}
}

func TestResponsePolicyUsesImmutableMaxAgeAcrossJSCSSEncodings(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	cfg.Frontend.ImmutableCache.MaxAge = "12h"
	policy := cachepolicy.NewResponsePolicyFromConfig(&cfg)
	for _, assetPath := range []string{"assets/app-deadbeef.js", "assets/app-deadbeef.mjs", "assets/style-deadbeef.css"} {
		assertFingerprintedEncodingControls(t, policy, assetPath, "public, max-age=43200, immutable")
	}
}

func assertFingerprintedEncodingControls(t *testing.T, policy cachepolicy.ResponsePolicy, assetPath, want string) {
	t.Helper()
	for _, encoding := range []string{"", "br", "gzip", "zstd"} {
		t.Run(assetPath+"/"+encoding, func(t *testing.T) {
			result := &resolver.Result{Asset: &catalog.Asset{Path: assetPath}}
			if encoding != "" {
				result.Variant = &catalog.Variant{Encoding: encoding}
			}
			if got := policy.CacheControl(result); got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

func TestResponsePolicyKeepsOtherFingerprintedMaxAge(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	policy := cachepolicy.NewResponsePolicyFromConfig(&cfg)
	result := &resolver.Result{Asset: &catalog.Asset{Path: "assets/hero-deadbeef.png"}}
	if got := policy.CacheControl(result); got != "public, max-age=31536000, immutable" {
		t.Fatalf("unexpected image cache-control %q", got)
	}
}

func TestResponsePolicyFingerprintCacheDisabledOrZero(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled bool
		maxAge  string
	}{
		{name: "disabled", enabled: false, maxAge: "12h"},
		{name: "zero", enabled: true, maxAge: "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.DefaultConfigForTest()
			cfg.Frontend.ImmutableCache.Enable = tc.enabled
			cfg.Frontend.ImmutableCache.MaxAge = tc.maxAge
			policy := cachepolicy.NewResponsePolicyFromConfig(&cfg)
			assertFingerprintedEncodingControls(t, policy, "assets/app-deadbeef.js", cachepolicy.RevalidateCacheControl)
		})
	}
}
