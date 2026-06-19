package config

type Debug struct {
	Enable      bool   `koanf:"enable"`
	PprofPrefix string `koanf:"pprof_prefix" validate:"omitempty,startswith=/"`
}
