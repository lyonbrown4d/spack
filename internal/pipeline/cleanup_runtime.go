package pipeline

import (
	"context"
	"sync"
	"time"

	"github.com/samber/oops"
)

type cleanupRuntime struct {
	interval time.Duration
	run      func(context.Context)
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

func newCleanupRuntime(interval time.Duration, run func(context.Context)) *cleanupRuntime {
	return &cleanupRuntime{
		interval: interval,
		run:      run,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

func (r *cleanupRuntime) Start(ctx context.Context) {
	go r.loop(ctx)
}

func (r *cleanupRuntime) Stop(ctx context.Context) error {
	r.stopOnce.Do(func() {
		close(r.stop)
	})
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		return oops.Wrapf(ctx.Err(), "wait for cleanup shutdown")
	}
}

func (r *cleanupRuntime) loop(ctx context.Context) {
	defer close(r.done)
	r.run(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stop:
			return
		case <-ticker.C:
			r.run(ctx)
		}
	}
}
