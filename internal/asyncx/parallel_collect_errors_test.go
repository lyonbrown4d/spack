package asyncx_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/samber/oops"
)

func TestRunListCollectErrorsPreservesEveryExecutedError(t *testing.T) {
	firstErr := errors.New("first failure")
	secondErr := errors.New("second failure")

	err := asyncx.RunListCollectErrors(
		t.Context(),
		nil,
		&asyncx.Settings{Size: 2},
		"collect_errors",
		cxlist.NewList(1, 2, 3),
		func(_ context.Context, value int) error {
			switch value {
			case 1:
				return oops.Wrapf(firstErr, "first task")
			case 2:
				return oops.Wrapf(secondErr, "second task")
			default:
				return nil
			}
		},
	)
	if !errors.Is(err, firstErr) {
		t.Fatalf("expected joined error to preserve first failure, got %v", err)
	}
	if !errors.Is(err, secondErr) {
		t.Fatalf("expected joined error to preserve second failure, got %v", err)
	}
}

func TestRunListCollectErrorsStopsSchedulingAfterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	started := make(chan struct{})
	var mu sync.Mutex
	executed := make([]int, 0, 1)
	go func() {
		<-started
		cancel()
	}()

	err := asyncx.RunListCollectErrors(
		ctx,
		nil,
		&asyncx.Settings{Size: 1},
		"collect_canceled",
		cxlist.NewList(1, 2, 3),
		func(ctx context.Context, value int) error {
			mu.Lock()
			executed = append(executed, value)
			mu.Unlock()
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(executed) != 1 || executed[0] != 1 {
		t.Fatalf("expected only first item to execute, got %v", executed)
	}
}

func TestRunListCollectErrorsReportsContextCauseOnce(t *testing.T) {
	const workers = 3
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	started := make(chan struct{}, workers)
	done := make(chan error, 1)
	go func() {
		done <- asyncx.RunListCollectErrors(
			ctx,
			nil,
			&asyncx.Settings{Size: workers},
			"collect_context_once",
			cxlist.NewList(1, 2, 3),
			func(ctx context.Context, _ int) error {
				started <- struct{}{}
				<-ctx.Done()
				return ctx.Err()
			},
		)
	}()

	for range workers {
		awaitSignal(t, started, cancel, "workers did not all start")
	}
	cancel()

	var err error
	select {
	case err = <-done:
	case <-time.After(time.Second):
		t.Fatal("collect-errors did not finish after context cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if got := strings.Count(err.Error(), context.Canceled.Error()); got != 1 {
		t.Fatalf("expected context cancellation once, got %d occurrences in %q", got, err)
	}
}

type parallelismProbe struct {
	current     atomic.Int64
	peak        atomic.Int64
	started     chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
}

func newParallelismProbe(limit int) *parallelismProbe {
	return &parallelismProbe{
		started: make(chan struct{}, limit+1),
		release: make(chan struct{}),
	}
}

func (p *parallelismProbe) run(_ context.Context, _ int) error {
	running := p.current.Add(1)
	defer p.current.Add(-1)
	p.recordPeak(running)
	p.started <- struct{}{}
	<-p.release
	return nil
}

func (p *parallelismProbe) recordPeak(running int64) {
	for observed := p.peak.Load(); running > observed; observed = p.peak.Load() {
		if p.peak.CompareAndSwap(observed, running) {
			return
		}
	}
}

func (p *parallelismProbe) releaseWorkers() {
	p.releaseOnce.Do(func() {
		close(p.release)
	})
}

func TestRunListCollectErrorsHonorsConfiguredParallelism(t *testing.T) {
	const limit = 3

	probe := newParallelismProbe(limit)
	t.Cleanup(probe.releaseWorkers)
	done := make(chan error, 1)
	go func() {
		done <- asyncx.RunListCollectErrors(
			t.Context(),
			nil,
			&asyncx.Settings{Size: limit},
			"bounded_collect_errors",
			cxlist.NewList(1, 2, 3, 4, 5, 6),
			probe.run,
		)
	}()

	for range limit {
		awaitSignal(t, probe.started, probe.releaseWorkers, "configured parallelism was not reached")
	}
	if got := probe.peak.Load(); got != limit {
		t.Fatalf("expected peak concurrency %d while workers are blocked, got %d", limit, got)
	}

	probe.releaseWorkers()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("collect-errors did not finish after workers were released")
	}
	if got := probe.peak.Load(); got > limit {
		t.Fatalf("expected peak concurrency at most %d, got %d", limit, got)
	}
	if got := probe.peak.Load(); got != limit {
		t.Fatalf("expected configured concurrency %d to be reached, got %d", limit, got)
	}
}

func awaitSignal[T any](t *testing.T, signal <-chan T, release func(), failure string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		release()
		t.Fatal(failure)
	}
}
