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
	if cacheControl != cachepolicy.RevalidateCacheControl {
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
