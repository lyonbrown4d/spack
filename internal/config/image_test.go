package config_test

import (
	"slices"
	"testing"

	"github.com/lyonbrown4d/spack/internal/config"
)

func TestImageParsedWidthsFiltersSortsAndDeduplicates(t *testing.T) {
	cfg := config.Image{Widths: "1280, 640, bad, 1280, 0, -1, 1920"}

	got := cfg.ParsedWidths()
	want := []int{640, 1280, 1920}
	if !slices.Equal(got.Values(), want) {
		t.Fatalf("expected widths %#v, got %#v", want, got)
	}
}

func TestDefaultImageConfigSetsSafeBuiltinEngineLimits(t *testing.T) {
	cfg := config.DefaultConfigForTest().Image

	if cfg.Engine != "builtin" {
		t.Fatalf("expected builtin image engine, got %q", cfg.Engine)
	}
	if cfg.MaxSourceBytes <= 0 {
		t.Fatalf("expected max source bytes limit, got %d", cfg.MaxSourceBytes)
	}
	if cfg.MaxSourcePixels <= 0 {
		t.Fatalf("expected max source pixels limit, got %d", cfg.MaxSourcePixels)
	}
	if cfg.MaxOutputVariants <= 0 {
		t.Fatalf("expected max output variants limit, got %d", cfg.MaxOutputVariants)
	}
	if cfg.MinSavingRatio <= 0 {
		t.Fatalf("expected low-benefit saving ratio, got %f", cfg.MinSavingRatio)
	}
	if cfg.MinSavingBytes <= 0 {
		t.Fatalf("expected low-benefit saving byte threshold, got %d", cfg.MinSavingBytes)
	}
}
