package asyncx

import (
	"context"
	"errors"
	"github.com/samber/oops"
	"strings"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/observabilityx"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"
)

var (
	asyncBatchItemsTotalSpec = observabilityx.NewCounterSpec(
		"async_batch_items_total",
		observabilityx.WithDescription("Total number of items submitted to asyncx batch runs."),
		observabilityx.WithLabelKeys("workload", "mode"),
	)
	asyncBatchRunsTotalSpec = observabilityx.NewCounterSpec(
		"async_batch_runs_total",
		observabilityx.WithDescription("Total number of asyncx batch executions."),
		observabilityx.WithLabelKeys("workload", "mode", "result"),
	)
	asyncBatchDurationSpec = observabilityx.NewHistogramSpec(
		"async_batch_duration_seconds",
		observabilityx.WithDescription("Asyncx batch execution duration in seconds."),
		observabilityx.WithUnit("s"),
		observabilityx.WithLabelKeys("workload", "mode", "result"),
	)
	asyncTaskRunsTotalSpec = observabilityx.NewCounterSpec(
		"async_task_runs_total",
		observabilityx.WithDescription("Total number of individual asyncx task executions."),
		observabilityx.WithLabelKeys("workload", "mode", "result"),
	)
	asyncTaskDurationSpec = observabilityx.NewHistogramSpec(
		"async_task_duration_seconds",
		observabilityx.WithDescription("Individual asyncx task execution duration in seconds."),
		observabilityx.WithUnit("s"),
		observabilityx.WithLabelKeys("workload", "mode", "result"),
	)
	asyncTaskSubmissionsTotalSpec = observabilityx.NewCounterSpec(
		"async_task_submissions_total",
		observabilityx.WithDescription("Total number of asyncx task submission attempts."),
		observabilityx.WithLabelKeys("workload", "result"),
	)
)

// RunList executes list items with the shared concurrency limit when available.
// It falls back to serial execution when the configured worker limit is one.
func RunList[T any](
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

	if run == nil || values.IsEmpty() {
		err := contextErr(ctx)
		recordBatchRunMetrics(ctx, obs, workload, mode, startedAt, err)
		return err
	}

	recordBatchItems(ctx, obs, workload, mode, values.Len())

	var err error
	if settings == nil || settings.Size <= 1 {
		err = runListSerial[T](ctx, obs, workload, mode, values, run)
	} else {
		err = runListParallel[T](ctx, obs, workload, mode, settings, values, run)
	}
	recordBatchRunMetrics(ctx, obs, workload, mode, startedAt, err)
	return err
}

func runListParallel[T any](
	ctx context.Context,
	obs observabilityx.Observability,
	workload string,
	mode string,
	settings *Settings,
	values *cxlist.List[T],
	run func(context.Context, T) error,
) error {
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(settings.Size)

	var scheduleErr error
	values.Range(func(_ int, value T) bool {
		if err := contextErr(groupCtx); err != nil {
			scheduleErr = err
			return false
		}

		recordTaskSubmission(groupCtx, obs, workload, true)
		group.Go(func() error {
			startedAt := time.Now()
			runErr := contextErr(groupCtx)
			if runErr == nil {
				runErr = run(groupCtx, value)
			}
			recordTaskRunMetrics(groupCtx, obs, workload, mode, startedAt, runErr)
			return runErr
		})
		return true
	})

	if err := group.Wait(); err != nil {
		return pickRunErr(ctx, err)
	}
	return pickRunErr(ctx, scheduleErr)
}

func runListSerial[T any](
	ctx context.Context,
	obs observabilityx.Observability,
	workload string,
	mode string,
	values *cxlist.List[T],
	run func(context.Context, T) error,
) error {
	var runErr error
	values.Range(func(_ int, value T) bool {
		startedAt := time.Now()
		runErr = run(ctx, value)
		recordTaskRunMetrics(ctx, obs, workload, mode, startedAt, runErr)
		return runErr == nil
	})
	return pickRunErr(ctx, runErr)
}

func pickRunErr(ctx context.Context, runErr error) error {
	if runErr != nil {
		return runErr
	}
	return contextErr(ctx)
}

func normalizeWorkload(workload string) string {
	workload = strings.TrimSpace(workload)
	if workload == "" {
		return "unknown"
	}
	return workload
}

func runMode(settings *Settings) string {
	if settings == nil || settings.Size <= 1 {
		return "serial"
	}
	return "parallel"
}

func asyncAttrs(workload, mode string) []observabilityx.Attribute {
	return []observabilityx.Attribute{
		observabilityx.String("workload", normalizeWorkload(workload)),
		observabilityx.String("mode", strings.TrimSpace(mode)),
	}
}

func recordBatchItems(ctx context.Context, obs observabilityx.Observability, workload, mode string, count int) {
	if count <= 0 {
		return
	}
	obs.Counter(asyncBatchItemsTotalSpec).Add(ctx, int64(count), asyncAttrs(workload, mode)...)
}

func recordBatchRunMetrics(
	ctx context.Context,
	obs observabilityx.Observability,
	workload string,
	mode string,
	startedAt time.Time,
	err error,
) {
	attrs := lo.Concat(asyncAttrs(workload, mode), []observabilityx.Attribute{observabilityx.String("result", metricResult(err))})
	obs.Counter(asyncBatchRunsTotalSpec).Add(ctx, 1, attrs...)
	obs.Histogram(asyncBatchDurationSpec).Record(ctx, time.Since(startedAt).Seconds(), attrs...)
}

func recordTaskRunMetrics(
	ctx context.Context,
	obs observabilityx.Observability,
	workload string,
	mode string,
	startedAt time.Time,
	err error,
) {
	attrs := lo.Concat(asyncAttrs(workload, mode), []observabilityx.Attribute{observabilityx.String("result", metricResult(err))})
	obs.Counter(asyncTaskRunsTotalSpec).Add(ctx, 1, attrs...)
	obs.Histogram(asyncTaskDurationSpec).Record(ctx, time.Since(startedAt).Seconds(), attrs...)
}

func recordTaskSubmission(ctx context.Context, obs observabilityx.Observability, workload string, submitted bool) {
	result := lo.Ternary(submitted, "submitted", "rejected")
	obs.Counter(asyncTaskSubmissionsTotalSpec).Add(ctx, 1,
		observabilityx.String("workload", normalizeWorkload(workload)),
		observabilityx.String("result", result),
	)
}

func metricResult(err error) string {
	if err == nil {
		return "ok"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "canceled"
	}
	return "error"
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return oops.Wrapf(err, "context done")
	}
	return nil
}
