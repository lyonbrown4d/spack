package cmd

import (
	"errors"
	"fmt"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/event"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/server"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

func Execute() error {
	return executeRoot(commandProfile{
		use:             "spack",
		short:           "Serve optimized frontend assets from a local directory or SPACK bundle.",
		enableRuntime:   true,
		enableCompiler:  true,
		enableUtilities: true,
	})
}

func ExecuteRuntime() error {
	return executeRoot(commandProfile{
		use:             "spack-runtime",
		short:           "Serve optimized frontend assets from a local directory or SPACK bundle.",
		enableRuntime:   true,
		enableUtilities: true,
	})
}

func ExecuteCompiler() error {
	return executeRoot(commandProfile{
		use:             "spack-compiler",
		short:           "Compile frontend assets into SPACK bundles.",
		enableCompiler:  true,
		enableUtilities: true,
	})
}

type commandProfile struct {
	use             string
	short           string
	enableRuntime   bool
	enableCompiler  bool
	enableUtilities bool
}

func executeRoot(profile commandProfile) error {
	root := newRootCommand(profile)
	if err := root.Execute(); err != nil {
		return oops.In("command").Wrap(fmt.Errorf("execute root command: %w", err))
	}
	return nil
}

func newRootCommand(profile commandProfile) *cobra.Command {
	command := &cobra.Command{
		Use:   profile.use,
		Short: profile.short,
	}
	bindConfigFlags(command)
	if profile.enableRuntime {
		bindRuntimeRoot(command)
	}
	if profile.enableUtilities {
		command.AddCommand(newConfigCommand(), newInspectCommand())
	}
	if profile.enableRuntime {
		command.AddCommand(newHealthcheckCommand())
	}
	if profile.enableCompiler {
		command.AddCommand(newCompileCommand())
	}
	if !profile.enableRuntime {
		command.RunE = func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q", args[0])
			}
			return cmd.Help()
		}
	}
	return command
}

func bindRuntimeRoot(command *cobra.Command) {
	var container *dix.App
	command.PreRunE = func(cmd *cobra.Command, args []string) error {
		dixInstance, err := createContainer(
			configLoadOptions(cmd),
			asyncx.Module,
			event.Module,
			source.Module,
			sourcecatalog.Module,
			contentcoding.Module,
			assetcache.Module,
			resolver.Module,
			server.Module,
		)
		if err != nil {
			return err
		}
		container = dixInstance
		return nil
	}
	command.RunE = func(cmd *cobra.Command, args []string) error {
		if container == nil {
			return errors.New("runtime container was not initialized")
		}
		return container.Run()
	}
	command.PostRun = func(cmd *cobra.Command, args []string) {
		if container == nil {
			return
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s", container.Meta()); err != nil {
			cmd.PrintErrln(err)
		}
	}
}
