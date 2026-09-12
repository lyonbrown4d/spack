package config

type Console struct {
	Enabled bool `configx:"usage=Enable console logging." json:"enabled" koanf:"enabled" yaml:"enabled"`
}

type File struct {
	Enabled  bool   `configx:"usage=Enable file logging."                           json:"enabled"   koanf:"enabled"   validate:"-"                        yaml:"enabled"`
	Path     string `configx:"usage=Log file path."                                 json:"path"      koanf:"path"      validate:"required_if=Enabled true" yaml:"path"`
	MaxSize  int    `configx:"usage=Maximum log file size before rotation."         json:"max_size"  koanf:"max_size"  validate:"gte=0"                    yaml:"max_size"`
	MaxAge   int    `configx:"usage=Maximum age in days for rotated log files."     json:"max_age"   koanf:"max_age"   validate:"gte=0"                    yaml:"max_age"`
	MaxFiles int    `configx:"usage=Maximum number of rotated log files to retain." json:"max_files" koanf:"max_files" validate:"gte=0"                    yaml:"max_files"`
}

type Logger struct {
	Level   string  `configx:"usage=Logger level." json:"level"    koanf:"level"  validate:"required,oneof=debug info warn error" yaml:"level"`
	Console Console `json:"console"                koanf:"console" yaml:"console"`
	File    File    `json:"file"                   koanf:"file"    yaml:"file"`
}
