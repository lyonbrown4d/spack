package task

import (
	"context"
	"log/slog"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/samber/oops"
)

const sourceRescanDebounce = 300 * time.Millisecond

func startSourceRescanWatcher(ctx context.Context, watcher *sourceRescanWatcher) error {
	if watcher == nil || watcher.runtime == nil || watcher.cancel != nil {
		return nil
	}

	watchCtx, cancel := context.WithCancel(ctx)
	changes, err := watcher.runtime.scanner.Watch(watchCtx)
	if err != nil {
		cancel()
		watcher.runtime.logger.Warn("Task source rescan watcher unavailable", slog.Any("error", err))
		return nil
	}

	watcher.cancel = cancel
	watcher.done = make(chan struct{})
	go watcher.watchSourceChanges(watchCtx, changes)
	watcher.runtime.logger.Info("Task source rescan watcher enabled", slog.Duration("debounce", sourceRescanDebounce))
	return nil
}

func (w *sourceRescanWatcher) watchSourceChanges(ctx context.Context, changes <-chan source.ChangeEvent) {
	defer close(w.done)

	timer := time.NewTimer(sourceRescanDebounce)
	stopTimer(timer)
	defer timer.Stop()

	pending := cxlist.NewDeque[source.ChangeEvent]()
	for {
		select {
		case <-ctx.Done():
			return
		case change, ok := <-changes:
			if !ok {
				return
			}
			w.recordPendingSourceChange(pending, change, timer)
		case <-timer.C:
			w.flushPendingSourceChanges(ctx, pending)
		}
	}
}

func (w *sourceRescanWatcher) recordPendingSourceChange(
	pending *cxlist.Deque[source.ChangeEvent],
	change source.ChangeEvent,
	timer *time.Timer,
) {
	if change.FullRescan {
		w.runtime.logger.Warn("Source watcher requested full rescan", slog.String("op", change.Op))
	} else {
		w.runtime.logger.Debug("Source change detected",
			slog.String("path", change.Path),
			slog.String("op", change.Op),
		)
	}
	pending.PushBack(change)
	resetTimer(timer, sourceRescanDebounce)
}

func (w *sourceRescanWatcher) flushPendingSourceChanges(ctx context.Context, pending *cxlist.Deque[source.ChangeEvent]) {
	if pending.IsEmpty() {
		return
	}
	events := pending.Values()
	pending.Clear()
	runSourceRescan(ctx, w.runtime, events...)
}
func stopSourceRescanWatcher(ctx context.Context, watcher *sourceRescanWatcher) error {
	if watcher == nil || watcher.cancel == nil {
		return nil
	}

	watcher.cancel()
	select {
	case <-watcher.done:
		watcher.cancel = nil
		watcher.done = nil
		return nil
	case <-ctx.Done():
		return oops.Wrapf(ctx.Err(), "stop source watcher")
	}
}

func resetTimer(timer *time.Timer, delay time.Duration) {
	stopTimer(timer)
	timer.Reset(delay)
}

func stopTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}
