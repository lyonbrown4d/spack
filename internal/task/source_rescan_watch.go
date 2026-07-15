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
		watcher.runtime.logger.Warn("Task source rescan watcher unavailable", slog.String("err", err.Error()))
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
			w.runtime.logger.Debug("Source change detected",
				slog.String("path", change.Path),
				slog.String("op", change.Op),
			)
			pending.PushBack(change)
			resetTimer(timer, sourceRescanDebounce)
		case <-timer.C:
			if pending.IsEmpty() {
				continue
			}
			events := pending.Values()
			pending.Clear()
			runSourceRescan(ctx, w.runtime, events...)
		}
	}
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
