package cmdruntime_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/spack/cmd/internal/cmdruntime"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

type utilityContextKey struct{}

func TestUtilityRunnerWrapCommandRunsCompleteLifecycle(t *testing.T) {
	t.Parallel()

	runner := cmdruntime.NewUtilityRunner()
	steps := make([]string, 0, 3)
	ctx := context.WithValue(context.Background(), utilityContextKey{}, "command")
	var startValue any
	var stopContextErr error
	module := dix.NewModule("test-lifecycle",
		dix.WithModuleHooks(
			dix.OnStart0(func(ctx context.Context) error {
				startValue = ctx.Value(utilityContextKey{})
				steps = append(steps, "start")
				return nil
			}),
			dix.OnStop0(func(ctx context.Context) error {
				stopContextErr = ctx.Err()
				steps = append(steps, "stop")
				return nil
			}),
		),
	)
	command := runner.WrapCommand(&cobra.Command{
		Use: "utility",
		RunE: func(*cobra.Command, []string) error {
			if _, err := runner.Start("test-utility", module); err != nil {
				return oops.Wrapf(err, "start utility runtime")
			}
			steps = append(steps, "command")
			return nil
		},
	})
	command.SetArgs(nil)

	if err := command.ExecuteContext(ctx); err != nil {
		t.Fatalf("execute utility command: %v", err)
	}
	if startValue != "command" {
		t.Fatalf("start context value = %v, want command", startValue)
	}
	if stopContextErr != nil {
		t.Fatalf("stop context error = %v, want nil", stopContextErr)
	}
	assertSteps(t, steps, "start", "command", "stop")
}

func TestUtilityRunnerRollsBackFailedSecondRuntimeAndStopsFirst(t *testing.T) {
	t.Parallel()

	startErr := errors.New("second runtime start failed")
	cleanupErr := errors.New("first runtime cleanup failed")
	steps := make([]string, 0, 5)
	runner := cmdruntime.NewUtilityRunner()
	first := lifecycleModule(
		"first",
		func() error {
			steps = append(steps, "start-a")
			return nil
		},
		func() error {
			steps = append(steps, "stop-a")
			return cleanupErr
		},
	)
	second := dix.NewModule("second",
		dix.WithModuleHooks(
			dix.OnStart0(func(context.Context) error {
				steps = append(steps, "start-b-ready")
				return nil
			}, dix.LifecycleName("ready"), dix.LifecyclePriority(10)),
			dix.OnStart0(func(context.Context) error {
				steps = append(steps, "start-b-fail")
				return startErr
			}, dix.LifecycleName("fail"), dix.LifecyclePriority(20)),
			dix.OnStop0(func(context.Context) error {
				steps = append(steps, "rollback-b")
				return nil
			}, dix.LifecycleName("ready"), dix.LifecyclePriority(10)),
		),
	)
	command := wrappedUtilityCommand(runner, func() error {
		if _, err := runner.Start("runtime-a", first); err != nil {
			return oops.Wrapf(err, "start utility runtime")
		}
		_, err := runner.Start("runtime-b", second)
		return oops.Wrapf(err, "start utility runtime")
	})

	err := command.ExecuteContext(context.Background())
	if !errors.Is(err, startErr) {
		t.Fatalf("execute error = %v, want start cause", err)
	}
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("execute error = %v, want cleanup cause", err)
	}
	assertSteps(t, steps, "start-a", "start-b-ready", "start-b-fail", "rollback-b", "stop-a")
}

func TestUtilityRunnerStopsRuntimesInLIFOOrderAndContinuesAfterFailure(t *testing.T) {
	t.Parallel()

	stopBErr := errors.New("stop b failed")
	steps := make([]string, 0, 5)
	runner := cmdruntime.NewUtilityRunner()
	command := wrappedUtilityCommand(runner, func() error {
		if _, err := runner.Start("runtime-a", lifecycleModule(
			"a",
			func() error {
				steps = append(steps, "start-a")
				return nil
			},
			func() error {
				steps = append(steps, "stop-a")
				return nil
			},
		)); err != nil {
			return oops.Wrapf(err, "start utility runtime")
		}
		if _, err := runner.Start("runtime-b", lifecycleModule(
			"b",
			func() error {
				steps = append(steps, "start-b")
				return nil
			},
			func() error {
				steps = append(steps, "stop-b")
				return stopBErr
			},
		)); err != nil {
			return oops.Wrapf(err, "start utility runtime")
		}
		steps = append(steps, "command")
		return nil
	})

	err := command.ExecuteContext(context.Background())
	if !errors.Is(err, stopBErr) {
		t.Fatalf("execute error = %v, want stop-b cause", err)
	}
	assertSteps(t, steps, "start-a", "start-b", "command", "stop-b", "stop-a")
}

func TestUtilityRunnerJoinsCommandAndMultipleStopErrors(t *testing.T) {
	t.Parallel()

	commandErr := errors.New("command failed")
	stopAErr := errors.New("stop a failed")
	stopBErr := errors.New("stop b failed")
	steps := make([]string, 0, 5)
	runner := cmdruntime.NewUtilityRunner()
	command := wrappedUtilityCommand(runner, func() error {
		if _, err := runner.Start("runtime-a", lifecycleModule(
			"a-errors",
			func() error {
				steps = append(steps, "start-a")
				return nil
			},
			func() error {
				steps = append(steps, "stop-a")
				return stopAErr
			},
		)); err != nil {
			return oops.Wrapf(err, "start utility runtime")
		}
		if _, err := runner.Start("runtime-b", lifecycleModule(
			"b-errors",
			func() error {
				steps = append(steps, "start-b")
				return nil
			},
			func() error {
				steps = append(steps, "stop-b")
				return stopBErr
			},
		)); err != nil {
			return oops.Wrapf(err, "start utility runtime")
		}
		steps = append(steps, "command")
		return commandErr
	})

	err := command.ExecuteContext(context.Background())
	for name, cause := range map[string]error{
		"command": commandErr,
		"stop-a":  stopAErr,
		"stop-b":  stopBErr,
	} {
		if !errors.Is(err, cause) {
			t.Fatalf("execute error = %v, want %s cause", err, name)
		}
	}
	assertSteps(t, steps, "start-a", "start-b", "command", "stop-b", "stop-a")
}

func TestUtilityRunnerStopsWithContextDetachedFromCommandCancellation(t *testing.T) {
	t.Parallel()

	runner := cmdruntime.NewUtilityRunner()
	ctx, cancel := context.WithCancel(context.Background())
	var stopContextErr error
	module := dix.NewModule("test-cancellation",
		dix.WithModuleHooks(
			dix.OnStop0(func(ctx context.Context) error {
				stopContextErr = ctx.Err()
				return nil
			}),
		),
	)
	command := runner.WrapCommand(&cobra.Command{
		Use: "utility",
		RunE: func(*cobra.Command, []string) error {
			if _, err := runner.Start("test-utility", module); err != nil {
				return oops.Wrapf(err, "start utility runtime")
			}
			cancel()
			return ctx.Err()
		},
	})
	command.SetArgs(nil)

	err := command.ExecuteContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("execute error = %v, want context canceled", err)
	}
	if stopContextErr != nil {
		t.Fatalf("stop context error = %v, want nil", stopContextErr)
	}
}

func lifecycleModule(name string, start, stop func() error) dix.Module {
	return dix.NewModule(name,
		dix.WithModuleHooks(
			dix.OnStart0(func(context.Context) error { return start() }),
			dix.OnStop0(func(context.Context) error { return stop() }),
		),
	)
}

func wrappedUtilityCommand(runner *cmdruntime.UtilityRunner, run func() error) *cobra.Command {
	command := runner.WrapCommand(&cobra.Command{
		Use: "utility",
		RunE: func(*cobra.Command, []string) error {
			return run()
		},
	})
	command.SetArgs(nil)
	return command
}

func assertSteps(t *testing.T, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("lifecycle steps = %v, want %v", got, want)
	}
}
