//go:build spack_libvips

package pipeline

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/semaphore"
)

type imageMemoryBudget struct {
	limit int64
	slots *semaphore.Weighted
}

func newImageMemoryBudget(limit int64) *imageMemoryBudget {
	if limit <= 0 {
		return nil
	}
	return &imageMemoryBudget{
		limit: limit,
		slots: semaphore.NewWeighted(limit),
	}
}

func (b *imageMemoryBudget) Acquire(bytes int64) (func(), error) {
	return b.AcquireContext(context.Background(), bytes)
}

func (b *imageMemoryBudget) AcquireContext(ctx context.Context, bytes int64) (func(), error) {
	if b == nil || bytes <= 0 {
		return noopMemoryRelease, nil
	}
	if bytes > b.limit {
		return noopMemoryRelease, fmt.Errorf(
			"estimated image memory bytes %d exceed max memory bytes %d: %w",
			bytes,
			b.limit,
			ErrVariantSkipped,
		)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := b.slots.Acquire(ctx, bytes); err != nil {
		return noopMemoryRelease, fmt.Errorf("acquire image memory budget: %w", err)
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			b.slots.Release(bytes)
		})
	}, nil
}

func noopMemoryRelease() {}
