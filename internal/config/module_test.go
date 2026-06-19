package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcgolabs/configx"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/spf13/pflag"
)

func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()

	value, ok := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if !ok {
			if err := os.Unsetenv(key); err != nil {
				t.Fatalf("cleanup unset %s: %v", key, err)
			}
			return
		}
		t.Setenv(key, value)
	})
}

func TestLoadWithOptions_RejectsInvalidCompressionCachePolicyDurations(t *testing.T) {
	t.Helper()

	unsetEnvForTest(t, "SPACK_ASSETS_ROOT")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "spack.yaml")
	configBody := "" +
		"assets:\n" +
		"  root: ./dist\n" +
		"compression:\n" +
		"  cleanup_every: nope\n" +
		"  max_age: later\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := config.LoadWithOptions(config.LoadOptions{Files: []string{configPath}})
	if err == nil {
		t.Fatal("expected invalid compression durations to fail validation")
	}
	if !strings.Contains(err.Error(), "spack_duration") && !strings.Contains(err.Error(), "spack_flexible_duration") {
		t.Fatalf("expected validator error for compression duration, got %v", err)
	}
}

func TestLoadWithOptions_RejectsFallbackWithoutTarget(t *testing.T) {
	t.Helper()

	unsetEnvForTest(t, "SPACK_ASSETS_ROOT")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "spack.yaml")
	configBody := "" +
		"assets:\n" +
		"  root: ./dist\n" +
		"  fallback:\n" +
		"    on: not_found\n" +
		"    target: \"\"\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := config.LoadWithOptions(config.LoadOptions{Files: []string{configPath}})
	if err == nil {
		t.Fatal("expected fallback without target to fail validation")
	}
	if !strings.Contains(err.Error(), "required_with") {
		t.Fatalf("expected fallback target validation error, got %v", err)
	}
}

func TestLoadIntoDefaultConfigPreservesNestedDefaultsWithPartialDotenv(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	envBody := "APP_ASSETS_ROOT=/tmp/assets\nAPP_ASSETS_PATH=/\nAPP_COMPRESSION_ENABLE=true\n"
	if err := os.WriteFile(envPath, []byte(envBody), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfigForTest()
	if err := configx.Load(
		&cfg,
		configx.WithEnvPrefix("APP"),
		configx.WithIgnoreDotenvError(false),
		configx.WithDotenv(envPath),
	); err != nil {
		t.Fatal(err)
	}

	if cfg.Assets.Entry != "index.html" {
		t.Fatalf("expected default assets entry to be preserved, got %q", cfg.Assets.Entry)
	}
	if cfg.Assets.Fallback.On != config.FallbackOnNotFound {
		t.Fatalf("expected default fallback mode %q, got %q", config.FallbackOnNotFound, cfg.Assets.Fallback.On)
	}
	if cfg.Compression.CacheDir == "" {
		t.Fatal("expected default compression cache dir to be preserved")
	}
	if cfg.Compression.Workers != 2 {
		t.Fatalf("expected default compression workers 2, got %d", cfg.Compression.Workers)
	}
}

func TestLoadWithOptions_PrioritizesFlagsOverEnvOverFiles(t *testing.T) {
	t.Helper()

	unsetPriorityPrecedenceEnv(t)
	configPath := writePriorityPrecedenceConfig(t)
	configurePriorityPrecedenceEnv(t)
	flags := newPriorityPrecedenceFlagSet(t)

	cfg, err := config.LoadWithOptions(config.LoadOptions{
		Files:   []string{configPath},
		FlagSet: flags,
	})
	if err != nil {
		t.Fatal(err)
	}

	assertPriorityPrecedenceConfig(t, cfg)
}

func TestLoadWithOptions_LoadsHCLConfig(t *testing.T) {
	t.Helper()

	cases := []hclConfigCase{
		{
			name:        "hcl",
			filename:    "spack.hcl",
			httpPort:    7001,
			assetsPath:  "/from-hcl",
			assetsRoot:  "/hcl-root",
			loggerLevel: "debug",
			httpPrefork: false,
		},
		{
			name:        "tf",
			filename:    "spack.tf",
			httpPort:    7002,
			assetsPath:  "/from-tf",
			assetsRoot:  "/tf-root",
			loggerLevel: "warn",
			httpPrefork: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertLoadsHCLConfigCase(t, tc)
		})
	}
}

func unsetPriorityPrecedenceEnv(t *testing.T) {
	t.Helper()

	unsetEnvForTest(t, "SPACK_HTTP_PORT")
	unsetEnvForTest(t, "SPACK_HTTP_LOW_MEMORY")
	unsetEnvForTest(t, "SPACK_HTTP_PREFORK")
	unsetEnvForTest(t, "SPACK_ASSETS_PATH")
	unsetEnvForTest(t, "SPACK_LOGGER_LEVEL")
}

func writePriorityPrecedenceConfig(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "spack.yaml")
	configBody := "" +
		"http:\n" +
		"  port: 7001\n" +
		"  prefork: false\n" +
		"assets:\n" +
		"  path: /from-file\n" +
		"  root: /file-root\n" +
		"logger:\n" +
		"  level: warn\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatal(err)
	}
	return configPath
}

func configurePriorityPrecedenceEnv(t *testing.T) {
	t.Helper()

	t.Setenv("SPACK_ASSETS_ROOT", "/env-root")
	t.Setenv("SPACK_HTTP_PREFORK", "true")
}

func newPriorityPrecedenceFlagSet(t *testing.T) *pflag.FlagSet {
	t.Helper()

	flags := pflag.NewFlagSet("spack-test", pflag.ContinueOnError)
	flags.Int("http.port", 0, "")
	flags.Bool("http.low_memory", true, "")
	flags.Bool("http.prefork", false, "")
	flags.String("logger.level", "", "")
	if err := flags.Parse([]string{"--http.port=8088", "--http.low_memory=false", "--http.prefork=false", "--logger.level=debug"}); err != nil {
		t.Fatal(err)
	}
	return flags
}

func assertPriorityPrecedenceConfig(t *testing.T, cfg *config.Config) {
	t.Helper()

	if cfg.HTTP.Port != 8088 {
		t.Fatalf("expected flag to override http.port to 8088, got %d", cfg.HTTP.Port)
	}
	if cfg.HTTP.LowMemory {
		t.Fatal("expected flag to override http.low_memory to false")
	}
	if cfg.HTTP.Prefork {
		t.Fatal("expected flag to override http.prefork to false")
	}
	if cfg.Assets.Path != "/from-file" {
		t.Fatalf("expected config file to set assets.path, got %q", cfg.Assets.Path)
	}
	if cfg.Assets.Root != "/env-root" {
		t.Fatalf("expected env to override assets.root, got %q", cfg.Assets.Root)
	}
	if cfg.Logger.Level != "debug" {
		t.Fatalf("expected flag to override logger.level, got %q", cfg.Logger.Level)
	}
}
