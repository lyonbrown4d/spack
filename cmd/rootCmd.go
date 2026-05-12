package cmd

import (
	"fmt"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/spack/internal/artifact"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/event"
	"github.com/lyonbrown4d/spack/internal/pipeline"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/server"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var container *dix.App

var rootCmd = &cobra.Command{
	Use: "spack",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		dixInstance, err := createContainer(
			configLoadOptions(cmd),
			asyncx.Module,
			event.Module,
			source.Module,
			sourcecatalog.Module,
			artifact.Module,
			contentcoding.Module,
			assetcache.Module,
			pipeline.Module,
			resolver.Module,
			server.Module,
		)
		if err != nil {
			return err
		}
		container = dixInstance
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return container.Run()
	},
	PostRun: func(cmd *cobra.Command, args []string) {
		if _, err := fmt.Printf("%s", container.Meta()); err != nil {
			cmd.PrintErrln(err)
		}
	},
}

func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		return oops.In("command").Wrap(fmt.Errorf("execute root command: %w", err))
	}
	return nil
}
