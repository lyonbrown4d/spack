package requestpath_test

import (
	"testing"

	"github.com/lyonbrown4d/spack/internal/requestpath"
)

func BenchmarkCleanStaticAssetPath(b *testing.B) {
	const raw = "assets/app-BecOYeVz.js"

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		cleaned := requestpath.Clean(raw)
		if cleaned.Value != raw || cleaned.AllowsEntryFallback {
			b.Fatalf("unexpected cleaned path: %#v", cleaned)
		}
	}
}

func BenchmarkCleanEncodedAssetPath(b *testing.B) {
	const raw = "assets/%E6%88%91%E7%9A%84%E8%AE%A2%E5%8D%95_inactive-BecOYeVz.js"

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		cleaned := requestpath.Clean(raw)
		if cleaned.Value == "" || cleaned.AllowsEntryFallback {
			b.Fatalf("unexpected cleaned path: %#v", cleaned)
		}
	}
}

func BenchmarkCleanRouteLikePath(b *testing.B) {
	const raw = "docs/order-center"

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		cleaned := requestpath.Clean(raw)
		if cleaned.Value != raw || !cleaned.AllowsEntryFallback {
			b.Fatalf("unexpected cleaned path: %#v", cleaned)
		}
	}
}
