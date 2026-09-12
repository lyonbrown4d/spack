package config

type Debug struct {
	Enable      bool   `configx:"usage=Enable debug endpoints."                                       koanf:"enable"`
	PprofPrefix string `configx:"usage=Optional prefix prepended before Fiber /debug/pprof handlers." koanf:"pprof_prefix" validate:"omitempty,startswith=/"`
}
