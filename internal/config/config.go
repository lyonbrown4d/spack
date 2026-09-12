package config

type Config struct {
	APIVersion  string      `configx:"nocli"     koanf:"apiVersion"  validate:"required,oneof=spack.io/v1alpha1"`
	Kind        string      `configx:"nocli"     koanf:"kind"        validate:"required,oneof=RuntimeConfig"`
	HTTP        HTTP        `koanf:"http"        validate:"required"`
	Assets      Assets      `koanf:"assets"      validate:"required"`
	Async       Async       `koanf:"async"       validate:"required"`
	Debug       Debug       `koanf:"debug"`
	Image       Image       `koanf:"image"       validate:"required"`
	Frontend    Frontend    `koanf:"frontend"    validate:"required"`
	Metrics     Metrics     `koanf:"metrics"     validate:"required"`
	Logger      Logger      `koanf:"logger"      validate:"required"`
	Robots      Robots      `koanf:"robots"      validate:"required"`
	Compression Compression `koanf:"compression" validate:"required"`
}

type Metrics struct {
	Enable bool   `configx:"usage=Enable Prometheus metrics endpoint and runtime collectors." koanf:"enable"`
	Prefix string `configx:"usage=Metrics endpoint path."                                     koanf:"prefix" validate:"required,startswith=/"`
}
