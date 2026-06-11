package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/arcgolabs/configx"
	confighcl "github.com/arcgolabs/configx/format/hcl"
	configjson "github.com/arcgolabs/configx/format/json"
	configtoml "github.com/arcgolabs/configx/format/toml"
	configyaml "github.com/arcgolabs/configx/format/yaml"
	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/observabilityx"
	"github.com/go-playground/validator/v10"
	"github.com/lyonbrown4d/spack/internal/validation"
	"github.com/samber/oops"
)

func NewModule(loadOptions LoadOptions) dix.Module {
	return dix.NewModule("config",
		dix.WithModuleProviders(
			dix.Value(loadOptions),
			dix.ProviderErr2(func(loadOptions LoadOptions, validate *validator.Validate) (*Config, error) {
				return loadConfig(loadOptions, validate, nil, nil)
			}),
			dix.Provider1(func(cfg *Config) *Debug { return &cfg.Debug }),
			dix.Provider1(func(cfg *Config) *Image { return &cfg.Image }),
			dix.Provider1(func(cfg *Config) *Frontend { return &cfg.Frontend }),
			dix.Provider1(func(cfg *Config) *Metrics { return &cfg.Metrics }),
			dix.Provider1(func(cfg *Config) *Logger { return &cfg.Logger }),
			dix.Provider1(func(cfg *Config) *Robots { return &cfg.Robots }),
			dix.Provider1(func(cfg *Config) *HTTP { return &cfg.HTTP }),
			dix.Provider1(func(cfg *Config) *Assets { return &cfg.Assets }),
			dix.Provider1(func(cfg *Config) *Async { return &cfg.Async }),
			dix.Provider1(func(cfg *Config) *Compression { return &cfg.Compression }),
		),
	)
}

func loadConfig(
	loadOptions LoadOptions,
	validate *validator.Validate,
	logger *slog.Logger,
	obs observabilityx.Observability,
) (*Config, error) {
	loaded := defaultConfig()
	if len(loadOptions.Files) > 0 {
		fileOnly := configx.New(
			configx.WithFiles(loadOptions.Files...),
			configx.WithPriority(configx.SourceFile),
			configyaml.WithYAMLSupport(),
			configjson.WithJSONSupport(),
			configtoml.WithTomlSupport(),
			confighcl.WithHCLSupport(),
		)
		fileConfig, fileErr := fileOnly.LoadConfig()
		if fileErr != nil {
			return nil, oops.In("config").Wrap(fmt.Errorf("load config: %w", fileErr))
		}
		if fileConfig.Exists("assets.fallback.target") &&
			strings.TrimSpace(fileConfig.GetString("assets.fallback.target")) == "" &&
			strings.TrimSpace(fileConfig.GetString("assets.fallback.on")) != "" {
			return nil, oops.In("config").Wrap(errors.New("load config: assets.fallback.target failed validation: required_with"))
		}
	}

	loader := configx.New(loadOptions.configxOptions(validate, logger, obs)...)
	raw, err := loader.LoadConfig()
	if err != nil {
		return nil, oops.In("config").Wrap(fmt.Errorf("load config: %w", err))
	}
	if err := raw.UnmarshalWithValidate("", &loaded); err != nil {
		return nil, oops.In("config").Wrap(fmt.Errorf("load config: %w", err))
	}
	return &loaded, nil
}

func loadConfigWithValidation(loadOptions LoadOptions) (*Config, error) {
	validate, err := validation.New()
	if err != nil {
		return nil, oops.In("config").Wrap(fmt.Errorf("build validator: %w", err))
	}
	return loadConfig(loadOptions, validate, nil, nil)
}

func LoadWithOptions(loadOptions LoadOptions) (*Config, error) {
	return loadConfigWithValidation(loadOptions)
}
