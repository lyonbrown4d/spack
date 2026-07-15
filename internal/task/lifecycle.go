package task

import (
	"context"
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/go-co-op/gocron/v2"
	"github.com/samber/oops"
)

func startTaskRuntime(ctx context.Context, scheduler gocron.Scheduler, registrations *cxlist.List[taskRegistration]) error {
	return startScheduledTasks(ctx, scheduler, registrations)
}

func stopTaskRuntime(ctx context.Context, scheduler gocron.Scheduler) error {
	if err := scheduler.ShutdownWithContext(ctx); err != nil {
		return oops.In("task").Owner("scheduler").Wrap(err)
	}
	return nil
}
