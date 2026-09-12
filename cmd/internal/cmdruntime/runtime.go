// Package cmdruntime provides shared command-side DI runtime helpers.
package cmdruntime

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/mapper"
	"github.com/lyonbrown4d/spack/internal/cmdkit"
	configcmd "github.com/lyonbrown4d/spack/internal/commands/config"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/mapx"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/lyonbrown4d/spack/internal/validation"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

type startedUtilityRuntime struct {
	name        string
	runtime     *dix.Runtime
	stopTimeout time.Duration
}

// UtilityRunner owns the DIX runtimes started while one utility command executes.
type UtilityRunner struct {
	commandContext context.Context
	active         bool
	runtimes       []startedUtilityRuntime
}

// NewUtilityRunner returns a command-scoped utility runtime runner.
func NewUtilityRunner() *UtilityRunner {
	return &UtilityRunner{}
}

// WrapCommand applies utility lifecycle management to each executable command.
func (r *UtilityRunner) WrapCommand(command *cobra.Command) *cobra.Command {
	if command == nil {
		return nil
	}
	for _, child := range command.Commands() {
		r.WrapCommand(child)
	}
	if command.RunE == nil {
		return command
	}
	run := command.RunE
	command.RunE = func(cmd *cobra.Command, args []string) error {
		return r.execute(cmd.Context(), func() error {
			return run(cmd, args)
		})
	}
	return command
}

func (r *UtilityRunner) execute(ctx context.Context, command func() error) error {
	if r == nil {
		return oops.In("cmdruntime").Owner("utility").Errorf("utility runner is required")
	}
	if r.active {
		return oops.In("cmdruntime").Owner("utility").Errorf("utility runner is already executing")
	}
	r.active = true
	r.commandContext = ctx
	defer func() {
		r.active = false
		r.commandContext = nil
		r.runtimes = nil
	}()

	commandErr := command()
	stopErr := r.stop(ctx)
	switch {
	case commandErr == nil:
		return stopErr
	case stopErr == nil:
		return commandErr
	default:
		return oops.Wrapf(oops.Join(commandErr, stopErr), "execute utility command")
	}
}

// Start builds and starts a DIX runtime owned by the current utility command.
func (r *UtilityRunner) Start(name string, modules ...dix.Module) (*dix.Runtime, error) {
	if r == nil || !r.active {
		return nil, oops.In("cmdruntime").Owner("utility").Errorf("utility runtime %q must start inside a wrapped command", name)
	}
	allModules := append([]dix.Module{mapx.Module}, modules...)
	app := dix.New(name, dix.Modules(allModules...))
	rt, err := app.Start(r.commandContext)
	if err != nil {
		return nil, oops.Wrapf(err, "start %s utility runtime", name)
	}
	r.runtimes = append(r.runtimes, startedUtilityRuntime{
		name:        name,
		runtime:     rt,
		stopTimeout: app.RunStopTimeout(),
	})
	return rt, nil
}

func (r *UtilityRunner) stop(ctx context.Context) error {
	var stopErrors []error
	for _, started := range slices.Backward(r.runtimes) {
		stopCtx := context.WithoutCancel(ctx)
		cancel := func() {}
		if started.stopTimeout > 0 {
			stopCtx, cancel = context.WithTimeout(stopCtx, started.stopTimeout)
		}
		err := started.runtime.Stop(stopCtx)
		cancel()
		if err != nil {
			stopErrors = append(stopErrors, oops.Wrapf(err, "stop %s utility runtime", started.name))
		}
	}
	if len(stopErrors) == 0 {
		return nil
	}
	return oops.Wrapf(oops.Join(stopErrors...), "stop utility runtimes")
}

// ResolveConfigWithDix resolves effective configuration in the current utility runtime.
func (r *UtilityRunner) ResolveConfigWithDix(loadOptions config.LoadOptions) (*config.Config, error) {
	flags, err := cmdkit.CloneVisitedConfigFlags(loadOptions.FlagSet)
	if err != nil {
		return nil, oops.Wrapf(err, "clone utility config flags")
	}
	if assetsFlag := loadOptions.FlagSet.Lookup("assets"); assetsFlag != nil && assetsFlag.Changed {
		assetsRoot, assetsErr := loadOptions.FlagSet.GetString("assets")
		if assetsErr != nil {
			return nil, oops.Wrapf(assetsErr, "read inspect assets flag")
		}
		if assetsErr := flags.Set("assets.root", assetsRoot); assetsErr != nil {
			return nil, oops.Wrapf(assetsErr, "set inspect assets root")
		}
	}
	loadOptions.FlagSet = flags
	rt, err := r.ResolveConfigRuntimeWithDix(loadOptions)
	if err != nil {
		return nil, err
	}
	return rt.Config, nil
}

// ResolveConfigRuntimeWithDix resolves config command dependencies in the current utility runtime.
func (r *UtilityRunner) ResolveConfigRuntimeWithDix(loadOptions config.LoadOptions) (configcmd.Runtime, error) {
	rt, err := r.Start(
		"spack-config",
		validation.Module,
		config.NewModule(loadOptions),
	)
	if err != nil {
		return configcmd.Runtime{}, err
	}
	cfg, err := dix.ResolveAs[*config.Config](rt.Container())
	if err != nil {
		return configcmd.Runtime{}, oops.Wrapf(err, "resolve config")
	}
	instance, err := dix.ResolveAs[*mapper.Mapper](rt.Container())
	if err != nil {
		return configcmd.Runtime{}, oops.Wrapf(err, "resolve mapper")
	}
	return configcmd.Runtime{Config: cfg, Mapper: instance}, nil
}

// ResolveScannerWithDix resolves an asset scanner in the current utility runtime.
func (r *UtilityRunner) ResolveScannerWithDix(cfg *config.Config) (sourcecatalog.Scanner, error) {
	rt, err := r.Start(
		"spack-inspect",
		InspectConfigModule(cfg),
		contentcoding.Module,
		source.Module,
		sourcecatalog.Module,
		spackbundle.Module,
	)
	if err != nil {
		return sourcecatalog.Scanner{}, err
	}
	scanner, err := dix.ResolveAs[sourcecatalog.Scanner](rt.Container())
	if err != nil {
		return sourcecatalog.Scanner{}, oops.Wrapf(err, "resolve source scanner")
	}
	return scanner, nil
}

func InspectConfigModule(cfg *config.Config) dix.Module {
	return dix.NewModule("inspect-config",
		dix.WithModuleProviders(
			dix.Value(cfg),
			dix.Provider1(func(cfg *config.Config) *config.Assets { return &cfg.Assets }),
			dix.Provider1(func(cfg *config.Config) *config.Async { return &cfg.Async }),
			dix.Provider1(func(cfg *config.Config) *config.Compression { return &cfg.Compression }),
			dix.Provider1(func(cfg *config.Config) *config.Image { return &cfg.Image }),
			dix.Provider0(func() *slog.Logger { return slog.New(slog.DiscardHandler) }),
		),
	)
}
