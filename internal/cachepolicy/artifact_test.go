package cachepolicy_test

import (
	"testing"
	"time"

	"github.com/daiyuang/spack/internal/cachepolicy"
	"github.com/daiyuang/spack/internal/config"
)

func TestArtifactPolicyFallsBackToDefaultMaxAge(t *testing.T) {
	policy := cachepolicy.NewArtifactPolicy(&config.Compression{
		MaxAge: "168h",
	})

	if got := policy.MaxAge("encoding"); got != 168*time.Hour {
		t.Fatalf("expected default max-age, got %s", got)
	}
}

func TestArtifactPolicyUsesNamespaceMaxAgeWhenPresent(t *testing.T) {
	policy := cachepolicy.NewArtifactPolicy(&config.Compression{
		MaxAge:         "168h",
		EncodingMaxAge: "24h",
	})

	if got := policy.MaxAge("encoding"); got != 24*time.Hour {
		t.Fatalf("expected namespace max-age, got %s", got)
	}
}

func TestArtifactPolicyFallsBackToDefaultMaxCacheBytes(t *testing.T) {
	policy := cachepolicy.NewArtifactPolicy(&config.Compression{
		MaxCacheBytes: 2048,
	})

	if got := policy.MaxCacheBytesForNamespace("encoding"); got != 2048 {
		t.Fatalf("expected namespace fallback max cache bytes, got %d", got)
	}
}

func TestArtifactPolicyUsesNamespaceMaxCacheBytesWhenPresent(t *testing.T) {
	policy := cachepolicy.NewArtifactPolicy(&config.Compression{
		MaxCacheBytes:         2048,
		EncodingMaxCacheBytes: 512,
	})

	if got := policy.MaxCacheBytesForNamespace("encoding"); got != 512 {
		t.Fatalf("expected namespace max cache bytes, got %d", got)
	}
}
