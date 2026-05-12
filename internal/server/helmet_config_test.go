package server_test

import (
	"testing"

	"github.com/lyonbrown4d/spack/internal/server"
)

func TestNewHelmetConfigRelaxesCrossOriginPolicies(t *testing.T) {
	cfg := server.NewHelmetConfigForTest()

	if cfg.CrossOriginEmbedderPolicy != "unsafe-none" {
		t.Fatalf("expected COEP unsafe-none, got %q", cfg.CrossOriginEmbedderPolicy)
	}
	if cfg.CrossOriginResourcePolicy != "cross-origin" {
		t.Fatalf("expected CORP cross-origin, got %q", cfg.CrossOriginResourcePolicy)
	}
	if cfg.CrossOriginOpenerPolicy != "same-origin" {
		t.Fatalf("expected COOP to remain same-origin, got %q", cfg.CrossOriginOpenerPolicy)
	}
}
