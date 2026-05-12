package requestpath_test

import (
	"testing"

	"github.com/lyonbrown4d/spack/internal/requestpath"
)

func TestCleanDecodesAndNormalizesAssetPath(t *testing.T) {
	cleaned := requestpath.Clean("%E6%88%91%E7%9A%84%E8%AE%A2%E5%8D%95_inactive-BecOYeVz.js")

	if cleaned.Value != "我的订单_inactive-BecOYeVz.js" {
		t.Fatalf("expected decoded asset path, got %q", cleaned.Value)
	}
	if cleaned.AllowsEntryFallback {
		t.Fatal("expected static asset path not to allow entry fallback")
	}
}

func TestCleanMountedRetainsNestedAssetsPathUnderRootMount(t *testing.T) {
	cleaned := requestpath.CleanMounted("/assets/%E7%88%B1%E8%BD%A6E%E6%97%8F-BDwtVsb9.png", "/")

	if cleaned.Value != "assets/爱车E族-BDwtVsb9.png" {
		t.Fatalf("expected normalized mounted path, got %q", cleaned.Value)
	}
	if cleaned.AllowsEntryFallback {
		t.Fatal("expected nested static asset path not to allow entry fallback")
	}
}

func TestCleanAllowsEntryFallbackForRouteLikePath(t *testing.T) {
	cleaned := requestpath.Clean("docs/order-center")

	if cleaned.Value != "docs/order-center" {
		t.Fatalf("expected route-like path, got %q", cleaned.Value)
	}
	if !cleaned.AllowsEntryFallback {
		t.Fatal("expected route-like path to allow entry fallback")
	}
}
