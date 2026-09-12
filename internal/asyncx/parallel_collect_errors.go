package asyncx

import (
	"context"
	"errors"
	"sync"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/observabilityx"
	"github.com/samber/oops"
)

// RunListCollectErrors executes all items unless the caller context is canceled
// and joins every error returned by an executed item. RunList retains its
// existing fail-fast behavior for callers that need sibling cancellation.
func RunListCollectErrors[T any](
	ctx context.Context,
	obs observabilityx.Observability,
	settings *Settings,
	workload string,
	values *cxlist.List[T],
	run func(context.Context, T) error,
) error {
	obs = observabilityx.Normalize(obs, nil)
	workload = normalizeWorkload(workload)
	mode := runMode(settings)
	startedAt := time.Now()

	if run == nil || values == nil || values.IsEmpty() {
		err := contextErr(ctx)
		recordBatchRunMetrics(ctx, obs, workload, mode, startedAt, err)
		return err
	}

	recordBatchItems(ctx, obs, workload, mode, values.Len())
	err := runListCollectErrors(ctx, obs, settings, workload, mode, values, run)
	recordBatchRunMetrics(ctx, obs, workload, mode, startedAt, err)
	return err
}

// RunListCollectErrorsWith executes a collectionx list using a reusable Runner
// and aggregates task failures instead of canceling siblings on the first error.
func RunListCollectErrorsWith[T any](
	ctx context.Context,
	runner Runner,
	values *cxlist.List[T],
	run func(context.Context, T) error,
) error {
	return RunListCollectErrors(ctx, runner.obs, runner.settings, runner.workload, values, run)
}

func runListCollectErrors[T any](
	ctx context.Context,
	obs observabilityx.Observability,
	settings *Settings,
	workload string,
	mode string,
	values *cxlist.List[T],
	run func(context.Context, T) error,
) error {
	if settings == nil || settings.Size <= 1 {
		return runListSerialCollectErrors(ctx, obs, workload, mode, values, run)
	}
	return runListParallelCollectErrors(ctx, obs, workload, mode, settings.Size, values, run)
}

type indexedValue[T any] struct {
	index int
	value T
}

func runListParallelCollectErrors[T any](
	ctx context.Context,
	obs observabilityx.Observability,
	workload string,
	mode string,
	limit int,
	values *cxlist.List[T],
	run func(context.Context, T) error,
) error {
	var errorsByIndex []error
	values.ViewValues(func(items []T) {
		errorsByIndex = make([]error, len(items))
		jobs := make(chan indexedValue[T])
		workerCount := min(max(limit, 1), len(items))
		workers := startCollectWorkers(
			ctx,
			obs,
			workload,
			mode,
			workerCount,
			jobs,
			errorsByIndex,
			run,
		)
		scheduleCollectJobs(ctx, obs, workload, items, jobs)
		close(jobs)
		workers.Wait()
	})
	return joinCollectedErrors(ctx, errorsByIndex)
}

func startCollectWorkers[T any](
	ctx context.Context,
	obs observabilityx.Observability,
	workload string,
	mode string,
	count int,
	jobs <-chan indexedValue[T],
	errorsByIndex []error,
	run func(context.Context, T) error,
) *sync.WaitGroup {
	workers := &sync.WaitGroup{}
	for range count {
		workers.Go(func() {
			for job := range jobs {
				runCollectedJob(ctx, obs, workload, mode, job, errorsByIndex, run)
			}
		})
	}
	return workers
}

func runCollectedJob[T any](
	ctx context.Context,
	obs observabilityx.Observability,
	workload string,
	mode string,
	job indexedValue[T],
	errorsByIndex []error,
	run func(context.Context, T) error,
) {
	startedAt := time.Now()
	runErr := contextErr(ctx)
	if runErr == nil {
		runErr = run(ctx, job.value)
	}
	errorsByIndex[job.index] = runErr
	recordTaskRunMetrics(ctx, obs, workload, mode, startedAt, runErr)
}

func scheduleCollectJobs[T any](
	ctx context.Context,
	obs observabilityx.Observability,
	workload string,
	items []T,
	jobs chan<- indexedValue[T],
) {
	done := contextDone(ctx)
	for index, value := range items {
		if contextErr(ctx) != nil {
			return
		}
		select {
		case <-done:
			return
		case jobs <- indexedValue[T]{index: index, value: value}:
			recordTaskSubmission(ctx, obs, workload, true)
		}
	}
}

func runListSerialCollectErrors[T any](
	ctx context.Context,
	obs observabilityx.Observability,
	workload string,
	mode string,
	values *cxlist.List[T],
	run func(context.Context, T) error,
) error {
	collected := cxlist.NewList[error]()
	values.Range(func(_ int, value T) bool {
		if contextErr(ctx) != nil {
			return false
		}
		startedAt := time.Now()
		runErr := run(ctx, value)
		recordTaskRunMetrics(ctx, obs, workload, mode, startedAt, runErr)
		if runErr != nil {
			collected.Add(runErr)
		}
		return true
	})

	var joined error
	collected.ViewValues(func(runErrors []error) {
		joined = joinCollectedErrors(ctx, runErrors)
	})
	return joined
}

type collectedContextCauses struct {
	canceled bool
	deadline bool
}

func (c *collectedContextCauses) shouldCollect(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return c.markCanceled()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return c.markDeadline()
	}
	return true
}

func (c *collectedContextCauses) markCanceled() bool {
	if c.canceled {
		return false
	}
	c.canceled = true
	return true
}

func (c *collectedContextCauses) markDeadline() bool {
	if c.deadline {
		return false
	}
	c.deadline = true
	return true
}

func appendCollectedError(
	collected **cxlist.List[error],
	causes *collectedContextCauses,
	err error,
) {
	if !causes.shouldCollect(err) {
		return
	}
	if *collected == nil {
		*collected = cxlist.NewList[error]()
	}
	(*collected).Add(err)
}

func joinCollectedErrors(ctx context.Context, runErrors []error) error {
	var collected *cxlist.List[error]
	causes := &collectedContextCauses{}
	for _, err := range runErrors {
		appendCollectedError(&collected, causes, err)
	}
	appendCollectedError(&collected, causes, contextErr(ctx))
	if collected == nil {
		return nil
	}

	var joined error
	collected.ViewValues(func(runErrors []error) {
		joined = errors.Join(runErrors...)
	})
	return oops.Wrapf(joined, "run async batch")
}

func contextDone(ctx context.Context) <-chan struct{} {
	if ctx == nil {
		return nil
	}
	return ctx.Done()
}
