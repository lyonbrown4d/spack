package source

import (
	"log/slog"

	"github.com/lyonbrown4d/spack/internal/config"
)

// NewLocalFSForTest exposes local filesystem source construction for external tests.
func NewLocalFSForTest(cfg *config.Assets, logger *slog.Logger) (Source, error) {
	return newLocalFS(cfg, logger)
}

// NewSourceForTest exposes backend-based source construction for external tests.
func NewSourceForTest(cfg *config.Assets, logger *slog.Logger) (Source, error) {
	return newSourceFromConfig(cfg, logger)
}
