package pipeline

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/panjf2000/ants/v2"
	"github.com/samber/oops"
)

func (s *Service) startWorkers(ctx context.Context, workers int) error {
	workers = max(workers, 1)
	pool, err := ants.NewPoolWithFunc(
		workers,
		func(any) {
			s.runWorker(ctx)
		},
		ants.WithDisablePurge(true),
		ants.WithLogger(pipelineWorkerPoolLogger{logger: s.logger}),
		ants.WithPanicHandler(func(value any) {
			s.logger.Error("Pipeline worker panicked", slog.Any("panic", value))
		}),
	)
	if err != nil {
		return oops.Wrapf(err, "create pipeline worker pool")
	}

	s.lazyWorkerPool = pool
	for range workers {
		if err := pool.Invoke(struct{}{}); err != nil {
			s.closeTaskQueue()
			pool.Release()
			return oops.Wrapf(err, "start pipeline worker")
		}
	}
	return nil
}

func (s *Service) runWorker(ctx context.Context) {
	for request := range s.tasks {
		s.processQueuedRequest(ctx, request)
	}
}

func (s *Service) processQueuedRequest(ctx context.Context, request Request) {
	key := requestKey(request)
	s.updateQueueLengthMetric()
	defer s.finishRequest(key)
	s.process(ctx, request)
}

func (s *Service) stopWorkers(ctx context.Context) error {
	s.closeTaskQueue()
	if s.lazyWorkerPool == nil || s.lazyWorkerPool.IsClosed() {
		return nil
	}
	if err := s.lazyWorkerPool.ReleaseContext(ctx); err != nil {
		return oops.Wrapf(err, "wait for worker shutdown")
	}
	return nil
}

func (s *Service) closeTaskQueue() {
	s.workerStopOnce.Do(func() {
		close(s.tasks)
	})
}

type pipelineWorkerPoolLogger struct {
	logger *slog.Logger
}

func (l pipelineWorkerPoolLogger) Printf(format string, args ...any) {
	if l.logger == nil {
		return
	}
	l.logger.Debug("Pipeline worker pool", slog.String("msg", fmt.Sprintf(format, args...)))
}
