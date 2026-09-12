package pipeline

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/samber/oops"
)

type canceledWorkerStage struct{}

func (canceledWorkerStage) Name() string {
	return "test"
}

func (canceledWorkerStage) Plan(*catalog.Asset, Request) *cxlist.List[Task] {
	return nil
}

func (canceledWorkerStage) Execute(context.Context, Task, *catalog.Asset) (*catalog.Variant, error) {
	return nil, ErrVariantSkipped
}

type ordinaryFailureAfterCancellationStage struct {
	started chan struct{}
	release chan struct{}
	cause   error
}

func (s *ordinaryFailureAfterCancellationStage) Name() string {
	return "ordinary-failure"
}

func (s *ordinaryFailureAfterCancellationStage) Plan(*catalog.Asset, Request) *cxlist.List[Task] {
	close(s.started)
	<-s.release
	return cxlist.NewList(Task{})
}

func (s *ordinaryFailureAfterCancellationStage) Execute(
	context.Context,
	Task,
	*catalog.Asset,
) (*catalog.Variant, error) {
	return nil, s.cause
}

type cancellationRecordingObservability struct {
	observabilityx.Observability
	counterAttrs []observabilityx.Attribute
}

func newCancellationRecordingObservability() *cancellationRecordingObservability {
	return &cancellationRecordingObservability{Observability: observabilityx.Nop()}
}

func (o *cancellationRecordingObservability) Counter(observabilityx.CounterSpec) observabilityx.Counter {
	return cancellationRecordingCounter{obs: o}
}

type cancellationRecordingCounter struct {
	obs *cancellationRecordingObservability
}

func (c cancellationRecordingCounter) Add(
	_ context.Context,
	_ int64,
	attrs ...observabilityx.Attribute,
) {
	c.obs.counterAttrs = append(c.obs.counterAttrs, attrs...)
}

func TestRecordStageTaskErrorClassifiesCanceledWorkerContext(t *testing.T) {
	tests := []struct {
		name    string
		context func() (context.Context, context.CancelFunc)
		cause   error
	}{
		{
			name: "canceled",
			context: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return ctx, func() {}
			},
			cause: context.Canceled,
		},
		{
			name: "deadline",
			context: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			},
			cause: context.DeadlineExceeded,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := test.context()
			defer cancel()
			obs := newCancellationRecordingObservability()
			svc := &Service{
				logger: slog.New(slog.DiscardHandler),
				obs:    obs,
			}
			svc.recordStageTaskError(
				ctx,
				canceledWorkerStage{},
				&catalog.Asset{Path: "app.js"},
				time.Now(),
				oops.Wrapf(test.cause, "worker stopped"),
			)

			if !hasMetricResult(obs.counterAttrs, "canceled") {
				t.Fatalf("expected canceled result attribute, got %v", obs.counterAttrs)
			}
			if hasMetricResult(obs.counterAttrs, "error") {
				t.Fatalf("expected no error result attribute, got %v", obs.counterAttrs)
			}
		})
	}
}

func hasMetricResult(attrs []observabilityx.Attribute, value any) bool {
	for _, attr := range attrs {
		if attr.Key == "result" && attr.Value == value {
			return true
		}
	}
	return false
}

func awaitPipelineWorkerSignal(
	t *testing.T,
	signal <-chan struct{},
	onTimeout func(),
	message string,
) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		onTimeout()
		t.Fatal(message)
	}
}

func TestProcessQueuedRequestLogsShutdownCancellationAtDebug(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cat := catalog.NewInMemoryCatalog()
	if err := cat.UpsertAsset(&catalog.Asset{Path: "app.js"}); err != nil {
		t.Fatal(err)
	}
	svc := newServiceState(
		&config.Compression{},
		logger,
		cat,
		serviceDeps{},
		0,
	)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	svc.processQueuedRequest(ctx, Request{AssetPath: "app.js"})

	logged := output.String()
	if !strings.Contains(logged, "\"level\":\"DEBUG\"") ||
		!strings.Contains(logged, "\"msg\":\"Pipeline queued request canceled\"") {
		t.Fatalf("expected debug cancellation log, got %q", logged)
	}
	if strings.Contains(logged, "\"level\":\"ERROR\"") ||
		strings.Contains(logged, "\"msg\":\"Pipeline queued request failed\"") {
		t.Fatalf("expected no error-level shutdown failure, got %q", logged)
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("expected canceled worker context, got %v", ctx.Err())
	}
}

func TestProcessQueuedRequestKeepsOrdinaryFailureWhenWorkerContextCanceled(t *testing.T) {
	wantErr := errors.New("transform failed")
	stage := &ordinaryFailureAfterCancellationStage{
		started: make(chan struct{}),
		release: make(chan struct{}),
		cause:   wantErr,
	}
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	obs := newCancellationRecordingObservability()
	cat := catalog.NewInMemoryCatalog()
	if err := cat.UpsertAsset(&catalog.Asset{Path: "app.js"}); err != nil {
		t.Fatal(err)
	}
	svc := newServiceState(
		&config.Compression{},
		logger,
		cat,
		serviceDeps{
			stages: cxlist.NewList[Stage](stage),
			obs:    obs,
		},
		0,
	)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.processQueuedRequest(ctx, Request{AssetPath: "app.js"})
	}()

	awaitPipelineWorkerSignal(t, stage.started, func() { close(stage.release) }, "timed out waiting for pipeline stage")
	cancel()
	close(stage.release)
	awaitPipelineWorkerSignal(t, done, func() {}, "timed out waiting for queued request")

	if !hasMetricResult(obs.counterAttrs, "error") {
		t.Fatalf("expected error result attribute, got %v", obs.counterAttrs)
	}
	if hasMetricResult(obs.counterAttrs, "canceled") {
		t.Fatalf("expected no canceled result attribute, got %v", obs.counterAttrs)
	}
	logged := output.String()
	if !strings.Contains(logged, "\"level\":\"ERROR\"") ||
		!strings.Contains(logged, "\"msg\":\"Pipeline queued request failed\"") ||
		!strings.Contains(logged, wantErr.Error()) {
		t.Fatalf("expected ordinary failure error log, got %q", logged)
	}
	if strings.Contains(logged, "Pipeline queued request canceled") {
		t.Fatalf("expected ordinary failure not to be classified as cancellation, got %q", logged)
	}
}
