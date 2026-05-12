package config_test

import (
	"runtime"
	"testing"

	"github.com/lyonbrown4d/spack/internal/config"
)

func TestAsyncNormalizedWorkersUsesExplicitValue(t *testing.T) {
	cfg := config.Async{Workers: 6}
	if got := cfg.NormalizedWorkers(); got != 6 {
		t.Fatalf("expected normalized workers 6, got %d", got)
	}
}

func TestAsyncNormalizedWorkersFallsBackToNumCPU(t *testing.T) {
	cfg := config.Async{}
	if got := cfg.NormalizedWorkers(); got != max(runtime.NumCPU(), 1) {
		t.Fatalf("expected normalized workers to follow NumCPU, got %d", got)
	}
}
