package config

import (
	"context"
	"os"

	"github.com/arcgolabs/configx"
)

// configx's single-underscore ENV separator cannot represent koanf names that
// themselves contain underscores. Keep the published SPACK_* names working.
func newCachePolicyEnvSource() configx.ConfigSource {
	return configx.NewSource("spack-cache-policy-env", func(_ context.Context) (map[string]any, error) {
		values := make(map[string]any, 4)
		for envName, configPath := range map[string]string{
			"SPACK_FRONTEND_IMMUTABLE_CACHE_ENABLE":  "frontend.immutable_cache.enable",
			"SPACK_FRONTEND_IMMUTABLE_CACHE_MAX_AGE": "frontend.immutable_cache.max_age",
			"SPACK_COMPRESSION_MAX_AGE":              "compression.max_age",
			"SPACK_COMPRESSION_ENCODING_MAX_AGE":     "compression.encoding_max_age",
		} {
			if value, ok := os.LookupEnv(envName); ok {
				values[configPath] = value
			}
		}
		return values, nil
	})
}
