package runtime

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/samber/oops"
)

func TestStartMainHTTPRuntimeUsesBoundListenerWithoutRebinding(t *testing.T) {
	listener := newRuntimeTestListener(t)
	httpRuntime := newRuntimeTestHTTPRuntime(t, listener)

	if err := startMainHTTPRuntime(t.Context(), httpRuntime); err != nil {
		t.Fatalf("start HTTP runtime with bound listener: %v", err)
	}
	t.Cleanup(func() {
		stopRuntimeTestHTTPRuntime(t, httpRuntime)
	})

	dialer := net.Dialer{Timeout: time.Second}
	connection, err := dialer.DialContext(
		t.Context(),
		"tcp4",
		listener.Addr().String(),
	)
	if err != nil {
		t.Fatalf("connect to started HTTP runtime: %v", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("close HTTP runtime connection: %v", err)
	}
}

func TestStartMainHTTPRuntimeReturnsSynchronousOopsForListenFailure(t *testing.T) {
	listenErr := errors.New("sentinel listen failure")
	httpRuntime := newRuntimeTestHTTPRuntimeForPort(0)
	httpRuntime.state.listen = func(context.Context, string, string) (net.Listener, error) {
		return nil, listenErr
	}

	err := startMainHTTPRuntime(t.Context(), httpRuntime)
	if !errors.Is(err, listenErr) {
		t.Fatalf("start error = %v, want sentinel listen failure", err)
	}
	wrapped, ok := errors.AsType[oops.OopsError](err)
	if !ok {
		t.Fatalf("start error type = %T, want oops.OopsError", err)
	}
	if wrapped.Domain() != "runtime" {
		t.Fatalf("start error domain = %q, want runtime", wrapped.Domain())
	}
	if wrapped.Owner() != "http runtime" {
		t.Fatalf("start error owner = %q, want http runtime", wrapped.Owner())
	}
}

func TestMainHTTPRuntimeImmediateStopConverges(t *testing.T) {
	const iterations = 5

	for iteration := range iterations {
		listener := newControlledRuntimeTestListener(errors.New("unexpected accept failure"))
		httpRuntime := newRuntimeTestHTTPRuntime(t, listener)

		if err := startMainHTTPRuntime(t.Context(), httpRuntime); err != nil {
			t.Fatalf("iteration %d start HTTP runtime: %v", iteration, err)
		}

		stopCtx, cancel := context.WithTimeout(t.Context(), time.Second)
		err := stopMainHTTPRuntime(stopCtx, httpRuntime)
		cancel()
		if err != nil {
			t.Fatalf("iteration %d stop HTTP runtime: %v", iteration, err)
		}

		assertRuntimeStopped(t, iteration, httpRuntime)
		if err := listener.Close(); err != nil {
			t.Fatalf("iteration %d repeat listener close: %v", iteration, err)
		}
	}
}

func assertRuntimeStopped(
	t *testing.T,
	iteration int,
	httpRuntime mainHTTPRuntime,
) {
	t.Helper()

	select {
	case <-httpRuntime.state.done:
	default:
		t.Fatalf("iteration %d HTTP runtime goroutine has not exited", iteration)
	}
}

type controlledRuntimeTestListener struct {
	acceptErr  error
	failAccept chan struct{}
	closed     chan struct{}
	failOnce   sync.Once
	closeOnce  sync.Once
}

func newControlledRuntimeTestListener(acceptErr error) *controlledRuntimeTestListener {
	return &controlledRuntimeTestListener{
		acceptErr:  acceptErr,
		failAccept: make(chan struct{}),
		closed:     make(chan struct{}),
	}
}

func (l *controlledRuntimeTestListener) Accept() (net.Conn, error) {
	select {
	case <-l.failAccept:
		return nil, l.acceptErr
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *controlledRuntimeTestListener) Close() error {
	l.closeOnce.Do(func() {
		close(l.closed)
	})
	return nil
}

func (l *controlledRuntimeTestListener) Addr() net.Addr {
	return runtimeTestAddr("controlled")
}

func (l *controlledRuntimeTestListener) FailAccept() {
	l.failOnce.Do(func() {
		close(l.failAccept)
	})
}

type runtimeTestAddr string

func (a runtimeTestAddr) Network() string {
	return "tcp"
}

func (a runtimeTestAddr) String() string {
	return string(a)
}

func newRuntimeTestListener(t *testing.T) net.Listener {
	t.Helper()

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(t.Context(), "tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("bind HTTP runtime test listener: %v", err)
	}
	return listener
}

func newRuntimeTestHTTPRuntime(t *testing.T, listener net.Listener) mainHTTPRuntime {
	t.Helper()

	httpRuntime := newRuntimeTestHTTPRuntimeForPort(0)
	httpRuntime.state.listen = func(context.Context, string, string) (net.Listener, error) {
		return listener, nil
	}
	return httpRuntime
}

func newRuntimeTestHTTPRuntimeForPort(port int) mainHTTPRuntime {
	cfg := config.DefaultConfigForTest()
	cfg.HTTP.Port = port

	return newMainHTTPRuntime(
		fiber.New(),
		&cfg,
		catalog.NewInMemoryCatalog(),
		slog.New(slog.DiscardHandler),
		NewFatalSignal(),
	)
}

func stopRuntimeTestHTTPRuntime(t *testing.T, httpRuntime mainHTTPRuntime) {
	t.Helper()

	if httpRuntime.state == nil || httpRuntime.state.done == nil {
		return
	}
	select {
	case <-httpRuntime.state.done:
		return
	default:
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := stopMainHTTPRuntime(stopCtx, httpRuntime); err != nil {
		t.Errorf("stop HTTP runtime: %v", err)
	}
}

type closeErrorRuntimeTestListener struct {
	closeErr error
}

func (l *closeErrorRuntimeTestListener) Accept() (net.Conn, error) {
	return nil, net.ErrClosed
}

func (l *closeErrorRuntimeTestListener) Close() error {
	return l.closeErr
}

func (l *closeErrorRuntimeTestListener) Addr() net.Addr {
	return runtimeTestAddr("close-error")
}
