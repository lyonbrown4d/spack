package main

import (
	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/spack/cmd/internal/cmdruntime"
	"github.com/lyonbrown4d/spack/internal/artifact"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/lyonbrown4d/spack/internal/compiler"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/event"
	"github.com/lyonbrown4d/spack/internal/metrics"
	"github.com/lyonbrown4d/spack/internal/pipeline"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

func resolveCompilerRuntimeWithDix(loadOptions config.LoadOptions, assetsRoot string) (compiler.Runtime, error) {
	compileOptions, err := compileLoadOptions(assetsRoot, loadOptions)
	if err != nil {
		return compiler.Runtime{}, oops.Wrapf(err, "resolve compile load options")
	}
	cfg, err := cmdruntime.ResolveConfigWithDix(compileOptions)
	if err != nil {
		return compiler.Runtime{}, oops.Wrapf(err, "resolve compile config")
	}
	compilerCfg := compilerConfigForGeneration(cfg)
	rt, err := cmdruntime.BuildUtilityRuntime(
		"spack-compiler",
		cmdruntime.InspectConfigModule(compilerCfg),
		metrics.Module,
		catalog.Module,
		asyncx.Module,
		event.Module,
		contentcoding.Module,
		source.Module,
		sourcecatalog.Module,
		spackbundle.Module,
		artifact.Module,
		pipeline.Module,
	)
	if err != nil {
		return compiler.Runtime{}, oops.Wrapf(err, "build compiler utility runtime")
	}
	scanner, err := dix.ResolveAs[sourcecatalog.Scanner](rt.Container())
	if err != nil {
		return compiler.Runtime{}, oops.Wrapf(err, "resolve source scanner")
	}
	cat, err := dix.ResolveAs[catalog.Catalog](rt.Container())
	if err != nil {
		return compiler.Runtime{}, oops.Wrapf(err, "resolve catalog")
	}
	pipelineSvc, err := dix.ResolveAs[*pipeline.Service](rt.Container())
	if err != nil {
		return compiler.Runtime{}, oops.Wrapf(err, "resolve compiler pipeline")
	}
	bundleWriter, err := dix.ResolveAs[spackbundle.BundleWriter](rt.Container())
	if err != nil {
		return compiler.Runtime{}, oops.Wrapf(err, "resolve bundle writer")
	}
	return compiler.Runtime{
		Config:       compilerCfg,
		Scanner:      scanner,
		Catalog:      cat,
		Generator:    pipelineSvc,
		BundleWriter: bundleWriter,
	}, nil
}

func compilerConfigForGeneration(cfg *config.Config) *config.Config {
	if cfg == nil {
		return nil
	}
	compilerCfg := *cfg
	if compilerCfg.Compression.Enable && compilerCfg.Compression.NormalizedMode() != config.CompressionModeOff {
		compilerCfg.Compression.Mode = config.CompressionModeWarmup
	}
	return &compilerCfg
}

func compileLoadOptions(assetsRoot string, base config.LoadOptions) (config.LoadOptions, error) {
	flags, err := cmdkit.CloneVisitedConfigFlags(base.FlagSet)
	if err != nil {
		return config.LoadOptions{}, oops.Wrapf(err, "clone compile config flags")
	}
	if err := flags.Set("assets.root", assetsRoot); err != nil {
		return config.LoadOptions{}, oops.Wrapf(err, "set compile assets root")
	}
	return config.LoadOptions{
		Files:   lo.Clone(base.Files),
		FlagSet: flags,
	}, nil
}
