// Package cachepolicy centralizes runtime cache policy decisions.
package cachepolicy

import (
	cxset "github.com/arcgolabs/collectionx/set"
	"strings"
	"time"

	cxinterval "github.com/arcgolabs/collectionx/interval"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/media"
)

const robotsAssetPath = "robots.txt"

type MemoryEntryKind string

const (
	MemoryEntryKindAsset   MemoryEntryKind = "asset"
	MemoryEntryKindVariant MemoryEntryKind = "variant"
)

type MemoryUseCase string

const (
	MemoryUseCaseServe  MemoryUseCase = "serve"
	MemoryUseCaseWarm   MemoryUseCase = "warm"
	MemoryUseCaseEvent  MemoryUseCase = "event"
	MemoryUseCaseDirect MemoryUseCase = "direct"
)

type MemoryRequest struct {
	Path           string
	AssetPath      string
	Size           int64
	MediaType      string
	Encoding       string
	Format         string
	Width          int
	Kind           MemoryEntryKind
	UseCase        MemoryUseCase
	RangeRequested bool
}

// MemoryPolicy decides when an asset should be served from the in-memory body cache.
type MemoryPolicy interface {
	ShouldServe(request MemoryRequest) bool
	ShouldWarm(request MemoryRequest) bool
	TTL(request MemoryRequest) time.Duration
}

// StaticMemoryPolicy applies size and range-request admission rules from static config.
type StaticMemoryPolicy struct {
	maxFileSize   int64
	baseTTL       time.Duration
	priorityTTL   time.Duration
	variantTTL    time.Duration
	genericTTL    time.Duration
	smallFileSize int64
	largeFileSize int64
	textTTLScheme *cxinterval.RangeMap[int64, int64]
	priorityPaths *cxset.OrderedSet[string]
}

// NewMemoryPolicy builds a memory-cache admission policy from HTTP config.
func NewMemoryPolicy(cfg *config.Config) MemoryPolicy {
	if cfg == nil {
		return StaticMemoryPolicy{}
	}

	baseTTL := cfg.HTTP.MemoryCache.ParsedTTL()
	return StaticMemoryPolicy{
		maxFileSize:   cfg.HTTP.MemoryCache.MaxFileSize,
		baseTTL:       baseTTL,
		priorityTTL:   clampMemoryTTL(baseTTL*2, baseTTL, 30*time.Minute),
		variantTTL:    clampMemoryTTL(baseTTL+baseTTL/2, baseTTL, 20*time.Minute),
		genericTTL:    clampMemoryTTL(baseTTL/2, time.Minute, baseTTL),
		smallFileSize: memorySmallFileSize(cfg.HTTP.MemoryCache.MaxFileSize),
		largeFileSize: memoryLargeFileSize(cfg.HTTP.MemoryCache.MaxFileSize),
		textTTLScheme: newMemoryTextTTLScheme(cfg.HTTP.MemoryCache.MaxFileSize),
		priorityPaths: memoryPriorityPaths(cfg),
	}
}

func (p StaticMemoryPolicy) ShouldServe(request MemoryRequest) bool {
	if request.RangeRequested {
		return false
	}
	return request.Size >= 0 && request.Size <= p.maxFileSize
}

func (p StaticMemoryPolicy) ShouldWarm(request MemoryRequest) bool {
	if !p.ShouldServe(request) {
		return false
	}
	if p.isPriorityPath(request) {
		return true
	}
	if request.UseCase == MemoryUseCaseEvent {
		return p.isVariant(request)
	}
	if p.isVariant(request) {
		return request.Size <= p.largeFileSize
	}
	if !isTextLikeRequest(request) {
		return false
	}
	return request.Size <= p.smallFileSize || request.Size <= 0
}

func (p StaticMemoryPolicy) TTL(request MemoryRequest) time.Duration {
	if !p.ShouldServe(request) {
		return 0
	}
	switch {
	case p.isPriorityPath(request):
		return p.priorityTTL
	case p.isVariant(request):
		return p.variantTTL
	case isTextLikeRequest(request):
		return p.adjustTTLForSize(p.baseTTL, request.Size)
	default:
		return p.genericTTL
	}
}

func (p StaticMemoryPolicy) isPriorityPath(request MemoryRequest) bool {
	return p.priorityPaths.Contains(memorySubjectPath(request))
}

func (p StaticMemoryPolicy) isVariant(request MemoryRequest) bool {
	if request.Kind == MemoryEntryKindVariant {
		return true
	}
	return strings.TrimSpace(request.Encoding) != "" || strings.TrimSpace(request.Format) != "" || request.Width > 0
}

func memoryPriorityPaths(cfg *config.Config) *cxset.OrderedSet[string] {
	if cfg == nil {
		return cxset.NewOrderedSet[string]()
	}
	return cxset.NewOrderedSet[string](
		strings.TrimSpace(cfg.Assets.Entry),
		strings.TrimSpace(cfg.Assets.Fallback.Target),
		robotsAssetPath,
	)
}

func memorySubjectPath(request MemoryRequest) string {
	if subject := strings.TrimSpace(request.AssetPath); subject != "" {
		return subject
	}
	return strings.TrimSpace(request.Path)
}

func isTextLikeRequest(request MemoryRequest) bool {
	if media.IsTextLikeMediaType(request.MediaType) {
		return true
	}

	return media.IsTextLikeFileExtension(memorySubjectPath(request))
}

func (p StaticMemoryPolicy) adjustTTLForSize(ttl time.Duration, size int64) time.Duration {
	if ttl <= 0 {
		return ttl
	}
	if size <= 0 {
		return ttl
	}
	if multiplier, ok := p.textTTLScheme.Get(size); ok {
		return time.Duration(ttl.Nanoseconds()*multiplier/1000) * time.Nanosecond
	}
	return ttl
}

func newMemoryTextTTLScheme(maxFileSize int64) *cxinterval.RangeMap[int64, int64] {
	scheme := cxinterval.NewRangeMap[int64, int64]()
	if maxFileSize <= 0 {
		return scheme
	}

	maxExclusive := maxFileSize + 1
	_ = scheme.Put(1, maxExclusive, 1000)

	smallFileSize := memorySmallFileSize(maxFileSize)
	if smallFileSize > 0 {
		_ = scheme.Put(1, min(memoryClampToBounds(smallFileSize)+1, maxExclusive), 1250)
	}

	largeFileSize := memoryLargeFileSize(maxFileSize)
	if largeFileSize > 0 {
		_ = scheme.Put(largeFileSize, maxExclusive, 500)
	}
	return scheme
}

func memoryClampToBounds(size int64) int64 {
	if size < 1 {
		return 1
	}
	return size
}

func memorySmallFileSize(maxFileSize int64) int64 {
	if maxFileSize <= 0 {
		return 0
	}
	candidate := max(1, maxFileSize/4)
	return clampMemorySize(candidate, 1024, maxFileSize)
}

func memoryLargeFileSize(maxFileSize int64) int64 {
	if maxFileSize <= 0 {
		return 0
	}
	candidate := max(1, (maxFileSize*3)/4)
	return min(candidate, maxFileSize)
}

func clampMemorySize(value, minValue, maxValue int64) int64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func clampMemoryTTL(value, minTTL, maxTTL time.Duration) time.Duration {
	switch {
	case value <= 0:
		return minTTL
	case value < minTTL:
		return minTTL
	case maxTTL > 0 && value > maxTTL:
		return maxTTL
	default:
		return value
	}
}
