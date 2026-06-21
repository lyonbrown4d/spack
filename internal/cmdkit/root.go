package cmdkit

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

func Execute(command *cobra.Command) error {
	if err := command.Execute(); err != nil {
		return oops.In("command").Wrap(fmt.Errorf("execute root command: %w", err))
	}
	return nil
}

func BindRuntimeRoot(command *cobra.Command) {
	var container *dix.App
	command.PreRunE = func(cmd *cobra.Command, args []string) error {
		dixInstance, err := CreateContainer(
			ConfigLoadOptions(cmd),
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
			return fmt.Errorf("create runtime container: %w", err)
		}
		container = dixInstance
		return nil
	}
	command.RunE = func(cmd *cobra.Command, args []string) error {
		if container == nil {
			return errors.New("runtime container was not initialized")
		}
		if err := container.Run(); err != nil {
			return fmt.Errorf("run runtime container: %w", err)
		}
		return nil
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
