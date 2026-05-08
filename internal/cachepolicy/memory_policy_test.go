package cachepolicy_test

import (
	"testing"

	"github.com/daiyuang/spack/internal/cachepolicy"
	"github.com/daiyuang/spack/internal/config"
)

func TestMemoryPolicyPrioritizesEntryAssets(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	baseTTL := cfg.HTTP.MemoryCache.ParsedTTL()
	policy := cachepolicy.NewMemoryPolicy(&cfg)

	request := cachepolicy.MemoryRequest{
		Path:      "index.html",
		AssetPath: "index.html",
		Size:      1024,
		MediaType: "text/html; charset=utf-8",
		Kind:      cachepolicy.MemoryEntryKindAsset,
		UseCase:   cachepolicy.MemoryUseCaseWarm,
	}

	if !policy.ShouldServe(request) {
		t.Fatal("expected entry asset to be cacheable")
	}
	if !policy.ShouldWarm(request) {
		t.Fatal("expected entry asset to be warmable")
	}
	if ttl := policy.TTL(request); ttl <= baseTTL {
		t.Fatalf("expected entry asset ttl to exceed base ttl, got %s <= %s", ttl, baseTTL)
	}
}

func TestMemoryPolicyKeepsLowValueBinaryAssetsOutOfWarmup(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	baseTTL := cfg.HTTP.MemoryCache.ParsedTTL()
	policy := cachepolicy.NewMemoryPolicy(&cfg)

	request := cachepolicy.MemoryRequest{
		Path:      "logo.png",
		AssetPath: "logo.png",
		Size:      1024,
		MediaType: "image/png",
		Kind:      cachepolicy.MemoryEntryKindAsset,
		UseCase:   cachepolicy.MemoryUseCaseWarm,
	}

	if !policy.ShouldServe(request) {
		t.Fatal("expected small binary asset to remain cacheable on demand")
	}
	if policy.ShouldWarm(request) {
		t.Fatal("expected low-value binary asset to stay out of warmup")
	}
	if ttl := policy.TTL(request); ttl >= baseTTL {
		t.Fatalf("expected low-value binary ttl to be shorter than base ttl, got %s >= %s", ttl, baseTTL)
	}
}

func TestMemoryPolicyWarmsVariantsFromEvents(t *testing.T) {
	policy := cachepolicy.NewMemoryPolicy(new(config.DefaultConfigForTest()))

	request := cachepolicy.MemoryRequest{
		Path:      "app.js.br",
		AssetPath: "app.js",
		Size:      1024,
		Kind:      cachepolicy.MemoryEntryKindVariant,
		UseCase:   cachepolicy.MemoryUseCaseEvent,
	}

	if !policy.ShouldServe(request) {
		t.Fatal("expected generated variant to be cacheable")
	}
	if !policy.ShouldWarm(request) {
		t.Fatal("expected generated variant event to warm cache")
	}
	if ttl := policy.TTL(request); ttl <= 0 {
		t.Fatalf("expected generated variant ttl to be positive, got %s", ttl)
	}
}

func TestMemoryPolicyRejectsRangeRequests(t *testing.T) {
	policy := cachepolicy.NewMemoryPolicy(new(config.DefaultConfigForTest()))

	if policy.ShouldServe(cachepolicy.MemoryRequest{
		Path:           "index.html",
		AssetPath:      "index.html",
		Size:           1024,
		MediaType:      "text/html",
		Kind:           cachepolicy.MemoryEntryKindAsset,
		UseCase:        cachepolicy.MemoryUseCaseServe,
		RangeRequested: true,
	}) {
		t.Fatal("expected range request to bypass memory cache")
	}
}

func TestMemoryPolicyPrioritizesSmallTextFilesForWarmup(t *testing.T) {
	policy := cachepolicy.NewMemoryPolicy(new(config.DefaultConfigForTest()))

	request := cachepolicy.MemoryRequest{
		Path:      "app.js",
		AssetPath: "app.js",
		Size:      2048,
		MediaType: "text/plain",
		Kind:      cachepolicy.MemoryEntryKindAsset,
		UseCase:   cachepolicy.MemoryUseCaseWarm,
	}

	if !policy.ShouldWarm(request) {
		t.Fatal("expected small text asset to be warmable")
	}
}

func TestMemoryPolicySkipsLargeTextFilesForWarmup(t *testing.T) {
	policy := cachepolicy.NewMemoryPolicy(new(config.DefaultConfigForTest()))

	request := cachepolicy.MemoryRequest{
		Path:      "docs/guide.html",
		AssetPath: "docs/guide.html",
		Size:      50000,
		MediaType: "text/html; charset=utf-8",
		Kind:      cachepolicy.MemoryEntryKindAsset,
		UseCase:   cachepolicy.MemoryUseCaseWarm,
	}

	if policy.ShouldWarm(request) {
		t.Fatal("expected large text asset to stay out of warmup")
	}
}

func TestMemoryPolicyScalesTTLByAssetSize(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	baseTTL := cfg.HTTP.MemoryCache.ParsedTTL()
	policy := cachepolicy.NewMemoryPolicy(&cfg)

	small := policy.TTL(cachepolicy.MemoryRequest{
		Path:      "small.txt",
		AssetPath: "small.txt",
		Size:      2048,
		MediaType: "text/plain",
		Kind:      cachepolicy.MemoryEntryKindAsset,
		UseCase:   cachepolicy.MemoryUseCaseServe,
	})
	large := policy.TTL(cachepolicy.MemoryRequest{
		Path:      "large.txt",
		AssetPath: "large.txt",
		Size:      50000,
		MediaType: "text/plain",
		Kind:      cachepolicy.MemoryEntryKindAsset,
		UseCase:   cachepolicy.MemoryUseCaseServe,
	})
	if small <= baseTTL {
		t.Fatalf("expected small text asset ttl to increase over base, got %s <= %s", small, baseTTL)
	}
	if large >= baseTTL {
		t.Fatalf("expected large text asset ttl to scale down under base, got %s >= %s", large, baseTTL)
	}
}

func TestMemoryPolicyUsesDistinctTTLBands(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	policy := cachepolicy.NewMemoryPolicy(&cfg)

	baseTTL := cfg.HTTP.MemoryCache.ParsedTTL()
	smallThreshold := cfg.HTTP.MemoryCache.MaxFileSize / 4
	smallThreshold = max(1024, smallThreshold)
	smallThreshold = min(cfg.HTTP.MemoryCache.MaxFileSize, smallThreshold)
	large := cfg.HTTP.MemoryCache.MaxFileSize * 3 / 4
	large = min(cfg.HTTP.MemoryCache.MaxFileSize, large)
	if large <= 0 {
		t.Fatal("expected positive large threshold for ttl band assertions")
	}

	if got := policy.TTL(cachepolicy.MemoryRequest{
		Path:      "app.js",
		AssetPath: "app.js",
		Size:      0,
		MediaType: "text/plain",
		Kind:      cachepolicy.MemoryEntryKindAsset,
		UseCase:   cachepolicy.MemoryUseCaseServe,
	}); got != baseTTL {
		t.Fatalf("expected unknown size ttl to match base ttl, got %s", got)
	}

	if got := policy.TTL(cachepolicy.MemoryRequest{
		Path:      "app.js",
		AssetPath: "app.js",
		Size:      smallThreshold,
		MediaType: "text/plain",
		Kind:      cachepolicy.MemoryEntryKindAsset,
		UseCase:   cachepolicy.MemoryUseCaseServe,
	}); got <= baseTTL {
		t.Fatalf("expected small text ttl to increase, got %s <= %s", got, baseTTL)
	}

	if got := policy.TTL(cachepolicy.MemoryRequest{
		Path:      "app.js",
		AssetPath: "app.js",
		Size:      large,
		MediaType: "text/plain",
		Kind:      cachepolicy.MemoryEntryKindAsset,
		UseCase:   cachepolicy.MemoryUseCaseServe,
	}); got >= baseTTL {
		t.Fatalf("expected large text ttl to decrease, got %s >= %s", got, baseTTL)
	}
}
