package config

import "runtime"

type Async struct {
	Workers int `configx:"usage=Shared async concurrency limit." koanf:"workers" validate:"gte=0"`
}

func (a Async) NormalizedWorkers() int {
	if a.Workers > 0 {
		return a.Workers
	}
	return max(runtime.NumCPU(), 1)
}
