// Package logger configures structured logging.
package logger

import (
	"context"
	"fmt"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/logx"
	"github.com/daiyuang/spack/internal/config"
	"github.com/samber/oops"
	"log/slog"
)

var Module = dix.NewModule("logger",
	dix.WithModuleProviders(
		dix.ProviderErr1(Build),
	),
	dix.WithModuleHooks(
		dix.OnStop(func(ctx context.Context, logger *slog.Logger) error {
			return logx.Close(logger)
		}),
	),
)

func Build(cfg *config.Config) (*slog.Logger, error) {
	opts := cxlist.NewListWithCapacity[logx.Option](5,
		logx.WithLevelString(cfg.Logger.Level),
		logx.WithConsole(cfg.Logger.Console.Enabled),
		logx.WithCaller(true),
		logx.WithGlobalLogger(),
	)

	if cfg.Logger.File.Enabled {
		opts.Add(
			logx.WithFile(cfg.Logger.File.Path),
			logx.WithFileRotation(cfg.Logger.File.MaxSize, cfg.Logger.File.MaxAge, cfg.Logger.File.MaxFiles),
		)
	}

	logger, err := logx.New(opts.Values()...)
	if err != nil {
		return nil, oops.In("logger").Owner("config").Wrap(fmt.Errorf("build logger: %w", err))
	}

	logx.SetDefault(logger)
	return logger, nil
}
