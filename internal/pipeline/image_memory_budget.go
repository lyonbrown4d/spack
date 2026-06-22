//go:build spack_libvips

package pipeline

import (
	"context"
	"errors"
	"sync"

	"github.com/samber/oops"
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
	if ctx == nil {
		return noopMemoryRelease, oops.In("pipeline").Owner("image memory budget").
			Wrap(errors.New("image memory budget context is nil"))
	}
	if bytes > b.limit {
		return noopMemoryRelease, oops.Wrapf(
			ErrVariantSkipped,
			"estimated image memory bytes %d exceed max memory bytes %d",
			bytes,
			b.limit,
		)
	}
	if err := b.slots.Acquire(ctx, bytes); err != nil {
		return noopMemoryRelease, oops.Wrapf(err, "acquire image memory budget")
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			b.slots.Release(bytes)
		})
	}, nil
}

func noopMemoryRelease() {}
