package asyncx

import (
	"github.com/lyonbrown4d/spack/internal/config"
)

type Settings struct {
	Size int
}

func newSettings(cfg *config.Async) *Settings {
	if cfg == nil {
		return &Settings{Size: 1}
	}
	return &Settings{
		Size: cfg.NormalizedWorkers(),
	}
}
