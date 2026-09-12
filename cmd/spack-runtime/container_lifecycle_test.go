package main

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/arcgolabs/dix"
	spackruntime "github.com/lyonbrown4d/spack/internal/runtime"
)

func TestRunRuntimeContainerContextReturnsFatalCauseAndStops(t *testing.T) {
	fatalErr := errors.New("sentinel runtime fatal failure")
	fatalSignal := spackruntime.NewFatalSignal()
	probe := &runtimeStopProbe{}
	app := newRuntimeLifecycleTestApp(fatalSignal, probe)
	fatalSignal.Report(fatalErr)

	err := runRuntimeContainerContext(t.Context(), app)
	if !errors.Is(err, fatalErr) {
		t.Fatalf("runtime error = %v, want fatal cause", err)
	}
	if probe.stopCount != 1 {
		t.Fatalf("DIX runtime stop count = %d, want 1", probe.stopCount)
	}
}

func TestRunRuntimeContainerContextPreservesFatalAndDifferentStopCauses(t *testing.T) {
	fatalErr := errors.New("sentinel runtime fatal failure")
	stopErr := errors.New("sentinel runtime stop failure")
	fatalSignal := spackruntime.NewFatalSignal()
	probe := &runtimeStopProbe{stopErr: stopErr}
	app := newRuntimeLifecycleTestApp(fatalSignal, probe)
	fatalSignal.Report(fatalErr)

	err := runRuntimeContainerContext(t.Context(), app)
	if !errors.Is(err, fatalErr) {
		t.Fatalf("runtime error = %v, want fatal cause", err)
	}
	if !errors.Is(err, stopErr) {
		t.Fatalf("runtime error = %v, want stop cause", err)
	}
	if probe.stopCount != 1 {
		t.Fatalf("DIX runtime stop count = %d, want 1", probe.stopCount)
	}
}

func TestRunRuntimeContainerContextPreservesFatalReportedDuringStop(t *testing.T) {
	fatalErr := errors.New("sentinel fatal reported during stop")
	fatalSignal := spackruntime.NewFatalSignal()
	probe := &runtimeStopProbe{
		onStop: func() {
			fatalSignal.Report(fatalErr)
		},
	}
	app := newRuntimeLifecycleTestApp(fatalSignal, probe)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := runRuntimeContainerContext(ctx, app)
	if !errors.Is(err, fatalErr) {
		t.Fatalf("runtime error = %v, want fatal cause reported during stop", err)
	}
	if probe.stopCount != 1 {
		t.Fatalf("DIX runtime stop count = %d, want 1", probe.stopCount)
	}
}

func TestWaitForRuntimeExitReturnsNilForSignal(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := waitForRuntimeExit(ctx, spackruntime.NewFatalSignal()); err != nil {
		t.Fatalf("signal exit error = %v, want nil", err)
	}
}

func TestWaitForRuntimeExitPreservesFatalWhenSignalAndFatalAreReady(t *testing.T) {
	fatalErr := errors.New("sentinel simultaneous fatal failure")
	fatalSignal := spackruntime.NewFatalSignal()
	fatalSignal.Report(fatalErr)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	for range 100 {
		err := waitForRuntimeExit(ctx, fatalSignal)
		if !errors.Is(err, fatalErr) {
			t.Fatalf("simultaneous exit error = %v, want fatal cause", err)
		}
	}
}

func TestFatalSignalKeepsFirstReportedCause(t *testing.T) {
	firstErr := errors.New("sentinel first fatal failure")
	secondErr := errors.New("sentinel second fatal failure")
	fatalSignal := spackruntime.NewFatalSignal()

	fatalSignal.Report(firstErr)
	fatalSignal.Report(secondErr)

	err := fatalSignal.Err()
	if !errors.Is(err, firstErr) {
		t.Fatalf("fatal error = %v, want first cause", err)
	}
	if errors.Is(err, secondErr) {
		t.Fatalf("fatal error = %v, must not contain second cause", err)
	}
}

func newRuntimeLifecycleTestApp(
	fatalSignal *spackruntime.FatalSignal,
	probe *runtimeStopProbe,
) *dix.App {
	module := dix.NewModule(
		"runtime-fatal-test",
		dix.WithModuleProviders(
			dix.Provider(func() *spackruntime.FatalSignal { return fatalSignal }),
			dix.Provider(func() *runtimeStopProbe { return probe }),
		),
		dix.WithModuleHooks(
			dix.OnStop(func(context.Context, *runtimeStopProbe) error {
				probe.mu.Lock()
				probe.stopCount++
				onStop := probe.onStop
				stopErr := probe.stopErr
				probe.mu.Unlock()
				if onStop != nil {
					onStop()
				}
				return stopErr
			}),
		),
	)
	return dix.New("runtime-fatal-test", dix.Modules(module))
}

type runtimeStopProbe struct {
	mu        sync.Mutex
	stopCount int
	stopErr   error
	onStop    func()
}
