package pipeline

import (
	"fmt"
	"sync"
)

type imageMemoryBudget struct {
	cond  *sync.Cond
	limit int64
	used  int64
}

func newImageMemoryBudget(limit int64) *imageMemoryBudget {
	if limit <= 0 {
		return nil
	}
	return &imageMemoryBudget{
		cond:  sync.NewCond(&sync.Mutex{}),
		limit: limit,
	}
}

func (b *imageMemoryBudget) Acquire(bytes int64) (func(), error) {
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

	b.cond.L.Lock()
	defer b.cond.L.Unlock()
	for b.used+bytes > b.limit {
		b.cond.Wait()
	}
	b.used += bytes

	var once sync.Once
	return func() {
		once.Do(func() {
			b.cond.L.Lock()
			defer b.cond.L.Unlock()
			b.used -= bytes
			if b.used < 0 {
				b.used = 0
			}
			b.cond.Broadcast()
		})
	}, nil
}

func noopMemoryRelease() {}
