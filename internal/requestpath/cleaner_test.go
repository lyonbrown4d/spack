package requestpath_test

import (
	"strings"
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

func TestTrimMountRequiresPathSegmentBoundary(t *testing.T) {
	for _, tc := range []struct {
		name        string
		requestPath string
		mountPath   string
		want        string
	}{
		{
			name:        "exact mount",
			requestPath: "/assets",
			mountPath:   "/assets",
			want:        "",
		},
		{
			name:        "nested asset",
			requestPath: "/assets/app.js",
			mountPath:   "/assets",
			want:        "app.js",
		},
		{
			name:        "trailing slash mount",
			requestPath: "/assets/app.js",
			mountPath:   "/assets/",
			want:        "app.js",
		},
		{
			name:        "prefix asset sibling",
			requestPath: "/assetsx/app.js",
			mountPath:   "/assets",
			want:        "assetsx/app.js",
		},
		{
			name:        "numeric suffix sibling",
			requestPath: "/assets2/app.js",
			mountPath:   "/assets",
			want:        "assets2/app.js",
		},
		{
			name:        "hyphen suffix sibling",
			requestPath: "/assets-admin/app.js",
			mountPath:   "/assets",
			want:        "assets-admin/app.js",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := requestpath.TrimMount(tc.requestPath, tc.mountPath); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestMountPatternsRequirePathSegmentBoundary(t *testing.T) {
	patterns := requestpath.MountPatterns("/assets/")

	if len(patterns) != 2 {
		t.Fatalf("expected two mount patterns, got %#v", patterns)
	}
	if patterns[0] != "/assets" || patterns[1] != "/assets/*" {
		t.Fatalf("unexpected mount patterns %#v", patterns)
	}
}

func FuzzTrimMountRejectsPrefixSiblings(f *testing.F) {
	f.Add("x")
	f.Add("2")
	f.Add("-admin")

	f.Fuzz(func(t *testing.T, suffix string) {
		if suffix == "" || strings.Contains(suffix, "/") {
			t.Skip()
		}

		requestPath := "/assets" + suffix + "/app.js"
		want := strings.TrimPrefix(requestPath, "/")
		if got := requestpath.TrimMount(requestPath, "/assets"); got != want {
			t.Fatalf("expected prefix sibling %q not to be trimmed, got %q", requestPath, got)
		}
	})
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
