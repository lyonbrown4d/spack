package config_test

import (
	"strings"
	"testing"

	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/mapx"
	"go.yaml.in/yaml/v3"
)

func TestBuildEffectiveConfigRedactsLocalPaths(t *testing.T) {
	cfg := &config.Config{
		Assets: config.Assets{
			Root: "D:/secret/dist",
		},
		Logger: config.Logger{
			File: config.File{
				Path: "D:/secret/spack.log",
			},
		},
		Compression: config.Compression{
			CacheDir: "D:/secret/cache",
			Mode:     config.CompressionModeWarmup,
		},
	}

	effective, err := config.BuildEffectiveConfig(mapx.New(), cfg, true)
	if err != nil {
		t.Fatal(err)
	}

	if effective.Assets.Root != "REDACTED" {
		t.Fatalf("expected assets root to be redacted, got %q", effective.Assets.Root)
	}
	if effective.Logger.File.Path != "REDACTED" {
		t.Fatalf("expected logger file path to be redacted, got %q", effective.Logger.File.Path)
	}
	if effective.Compression.CacheDir != "REDACTED" {
		t.Fatalf("expected compression cache dir to be redacted, got %q", effective.Compression.CacheDir)
	}
}

func TestBuildEffectiveConfigKeepsLazyQueueCompatibilityFields(t *testing.T) {
	cfg := &config.Config{
		Compression: config.Compression{
			Mode:      config.CompressionModeLazy,
			QueueSize: 0,
		},
	}

	effective, err := config.BuildEffectiveConfig(mapx.New(), cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	body, err := yaml.Marshal(effective)
	if err != nil {
		t.Fatal(err)
	}
	output := string(body)

	if !strings.Contains(output, "queue_size: 0") {
		t.Fatalf("expected lazy queue_size compatibility field, got:\n%s", output)
	}
	if !strings.Contains(output, "queue_size_scope: legacy_runtime_enqueue_compatibility") {
		t.Fatalf("expected lazy queue_size_scope compatibility field, got:\n%s", output)
	}
	if effective.Compression.GenerationScope != "legacy_runtime_enqueue_compatibility" {
		t.Fatalf("expected lazy generation scope, got %q", effective.Compression.GenerationScope)
	}
}

func TestBuildEffectiveConfigOmitsQueueCompatibilityFieldsOutsideLazyMode(t *testing.T) {
	cfg := &config.Config{
		Compression: config.Compression{
			Mode:      config.CompressionModeWarmup,
			QueueSize: 128,
		},
	}

	effective, err := config.BuildEffectiveConfig(mapx.New(), cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	body, err := yaml.Marshal(effective)
	if err != nil {
		t.Fatal(err)
	}
	output := string(body)

	if strings.Contains(output, "queue_size:") {
		t.Fatalf("expected queue_size to be omitted outside lazy mode, got:\n%s", output)
	}
	if strings.Contains(output, "queue_size_scope:") {
		t.Fatalf("expected queue_size_scope to be omitted outside lazy mode, got:\n%s", output)
	}
	if effective.Compression.GenerationScope != "compiler_only" {
		t.Fatalf("expected warmup generation scope, got %q", effective.Compression.GenerationScope)
	}
}
