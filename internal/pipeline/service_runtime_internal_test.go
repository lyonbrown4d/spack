package pipeline

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
)

func TestLazyWorkerPoolDrainsQueueAndClearsPending(t *testing.T) {
	svc := newServiceState(
		&config.Compression{
			Enable: true,
			Mode:   config.CompressionModeLazy,
		},
		slog.New(slog.DiscardHandler),
		catalog.NewInMemoryCatalog(),
		serviceDeps{},
		1,
	)

	ctx := context.Background()
	if err := svc.startWorkers(ctx, 1); err != nil {
		t.Fatalf("start lazy worker pool: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := svc.stopWorkers(stopCtx); err != nil {
			t.Errorf("stop lazy worker pool: %v", err)
		}
	})

	request := Request{AssetPath: "missing.js"}
	svc.pending.Add(requestKey(request))
	svc.tasks <- request

	assertEventually(t, func() bool {
		return len(svc.tasks) == 0 && svc.pending.Len() == 0
	})
}

func assertEventually(t *testing.T, check func() bool) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(time.Millisecond)
	}

	t.Fatal("condition was not satisfied before deadline")
}
