package runtime

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestMainHTTPRuntimePublishesAsynchronousServeError(t *testing.T) {
	serveErr := errors.New("sentinel accept failure")
	listener := newControlledRuntimeTestListener(serveErr)
	httpRuntime := newRuntimeTestHTTPRuntime(t, listener)

	if err := startMainHTTPRuntime(t.Context(), httpRuntime); err != nil {
		t.Fatalf("start HTTP runtime: %v", err)
	}

	listener.FailAccept()

	waitForRuntimeFatalCause(t, httpRuntime, serveErr)
	waitForRuntimeDone(t, httpRuntime)

	stopCtx, cancelStop := context.WithTimeout(t.Context(), time.Second)
	defer cancelStop()
	if err := stopMainHTTPRuntime(stopCtx, httpRuntime); err != nil {
		t.Fatalf("stop HTTP runtime after reported fatal: %v", err)
	}

	if err := listener.Close(); err != nil {
		t.Fatalf("close listener after stop: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener repeatedly: %v", err)
	}

	select {
	case <-httpRuntime.state.done:
	default:
		t.Fatal("HTTP runtime goroutine has not exited")
	}
}

func TestMainHTTPRuntimeDoesNotPublishNetClosedAsFatal(t *testing.T) {
	listener := newControlledRuntimeTestListener(net.ErrClosed)
	httpRuntime := newRuntimeTestHTTPRuntime(t, listener)

	if err := startMainHTTPRuntime(t.Context(), httpRuntime); err != nil {
		t.Fatalf("start HTTP runtime: %v", err)
	}
	listener.FailAccept()

	select {
	case <-httpRuntime.state.done:
	case <-time.After(time.Second):
		t.Fatal("wait for HTTP runtime goroutine")
	}
	select {
	case <-httpRuntime.fatal.Done():
		t.Fatalf("normal listener close published fatal error: %v", httpRuntime.fatal.Err())
	default:
	}
	if err := stopMainHTTPRuntime(t.Context(), httpRuntime); err != nil {
		t.Fatalf("stop HTTP runtime after listener close: %v", err)
	}
}

func TestCloseMainHTTPListenerIgnoresNetClosed(t *testing.T) {
	listener := &closeErrorRuntimeTestListener{closeErr: net.ErrClosed}
	if err := closeMainHTTPListener(&mainHTTPRuntimeState{listener: listener}); err != nil {
		t.Fatalf("close listener with net.ErrClosed: %v", err)
	}
}

func TestCloseMainHTTPListenerPreservesUnexpectedCause(t *testing.T) {
	closeErr := errors.New("sentinel close failure")
	listener := &closeErrorRuntimeTestListener{closeErr: closeErr}
	err := closeMainHTTPListener(&mainHTTPRuntimeState{listener: listener})
	if !errors.Is(err, closeErr) {
		t.Fatalf("close error = %v, want sentinel close failure", err)
	}
}

func TestStartMainHTTPRuntimeCancellationBeforeServeUsesBoundedCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	listener := newControlledRuntimeTestListener(errors.New("unexpected accept failure"))
	httpRuntime := newRuntimeTestHTTPRuntime(t, listener)
	httpRuntime.state.startCleanupTimeout = 20 * time.Millisecond

	beforeServe := make(chan struct{})
	releaseBeforeServe := make(chan struct{})
	httpRuntime.state.beforeServe = func() error {
		close(beforeServe)
		<-releaseBeforeServe
		return nil
	}

	startErr := make(chan error, 1)
	go func() {
		startErr <- startMainHTTPRuntime(ctx, httpRuntime)
	}()

	select {
	case <-beforeServe:
	case <-time.After(time.Second):
		t.Fatal("wait for BeforeServe barrier")
	}
	cancel()

	var err error
	select {
	case err = <-startErr:
	case <-time.After(time.Second):
		t.Fatal("start did not respect bounded cleanup timeout")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("start error = %v, want context canceled", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("start error = %v, want cleanup deadline exceeded", err)
	}

	close(releaseBeforeServe)
	waitForRuntimeDone(t, httpRuntime)
}

func TestStartMainHTTPRuntimeContextCancellationClosesDone(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	listener := newControlledRuntimeTestListener(errors.New("unexpected accept failure"))
	httpRuntime := newRuntimeTestHTTPRuntimeForPort(0)
	httpRuntime.state.listen = func(context.Context, string, string) (net.Listener, error) {
		cancel()
		return listener, nil
	}

	err := startMainHTTPRuntime(ctx, httpRuntime)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("start error = %v, want context canceled", err)
	}
	select {
	case <-httpRuntime.state.done:
	default:
		t.Fatal("HTTP runtime done was not closed after start cancellation")
	}
}
func waitForRuntimeFatalCause(t *testing.T, runtime mainHTTPRuntime, want error) {
	t.Helper()
	select {
	case <-runtime.fatal.Done():
	case <-time.After(time.Second):
		t.Fatal("wait for HTTP runtime fatal signal")
	}
	if err := runtime.fatal.Err(); !errors.Is(err, want) {
		t.Fatalf("fatal error = %v, want %v", err, want)
	}
}

func waitForRuntimeDone(t *testing.T, runtime mainHTTPRuntime) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	select {
	case <-runtime.state.done:
	case <-ctx.Done():
		t.Fatalf("wait for HTTP runtime goroutine: %v", ctx.Err())
	}
}
