package config

import (
	"log/slog"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/configx"
	configjson "github.com/arcgolabs/configx/format/json"
	configtoml "github.com/arcgolabs/configx/format/toml"
	configyaml "github.com/arcgolabs/configx/format/yaml"
	"github.com/arcgolabs/observabilityx"
	"github.com/go-playground/validator/v10"
	"github.com/lyonbrown4d/spack/internal/constant"
	"github.com/spf13/pflag"
)

// LoadOptions controls which external config sources are consulted in addition
// to the built-in defaults, dotenv files, and environment variables.
type LoadOptions struct {
	Files   []string
	FlagSet *pflag.FlagSet
}

func (o LoadOptions) configxOptions(
	validate *validator.Validate,
	logger *slog.Logger,
	obs observabilityx.Observability,
) []configx.Option {
	options := cxlist.NewList[configx.Option](
		configx.WithEnvPrefix(constant.EnvPrefix),
		configx.WithIgnoreDotenvError(true),
		configx.WithDotenv(),
		configx.WithValidator(validate),
		configx.WithValidateLevel(configx.ValidateLevelStruct),
		configx.WithLogger(logger),
		configx.WithObservability(obs),
		configyaml.WithYAMLSupport(),
		configjson.WithJSONSupport(),
		configtoml.WithTomlSupport(),
	)
	if len(o.Files) > 0 {
		options.Add(configx.WithFiles(o.Files...))
	}
	if o.FlagSet != nil {
		options.Add(configx.WithFlagSet(o.FlagSet))
	}
	return options.Values()
}
