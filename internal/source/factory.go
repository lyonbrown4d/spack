package source

import (
	"context"
	"errors"
	"log/slog"

	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/samber/oops"
)

type SourceFactory struct {
	resolver *Resolver
	logger   *slog.Logger
}

func NewSourceFactory(resolver *Resolver, logger *slog.Logger) *SourceFactory {
	if resolver == nil {
		resolver = NewResolver()
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &SourceFactory{
		resolver: resolver,
		logger:   logger,
	}
}

func (f *SourceFactory) LocalFS(cfg *config.Assets) (*LocalFS, error) {
	return f.LocalFSContext(context.TODO(), cfg)
}

func (f *SourceFactory) LocalFSContext(ctx context.Context, cfg *config.Assets) (*LocalFS, error) {
	if ctx == nil {
		return nil, oops.Owner("source").Wrap(errSourceContextNil)
	}
	if cfg == nil {
		return nil, oops.Owner("source").Wrap(errors.New("assets root is required"))
	}
	resolvedSource, err := f.resolver.Resolve(cfg.Root)
	if err != nil {
		return nil, err
	}
	resolved, err := resolveLocalFSResolvedRoot(ctx, resolvedSource)
	if err != nil {
		return nil, err
	}
	logSourceConfigured(f.logger, cfg.Root, resolved)
	return &LocalFS{
		root:                     resolved.root,
		rootInfo:                 resolved.info,
		logger:                   f.logger,
		bundle:                   resolved.bundle,
		cleanupRoot:              resolved.cleanupRoot,
		bundleExtractionDuration: resolved.bundleExtractionDuration,
	}, nil
}
