package config_test

import (
	"testing"

	"github.com/lyonbrown4d/spack/internal/config"
)

func TestLoadWithOptions_DockerEnvironmentEnablesStaleAssetRecovery(t *testing.T) {
	t.Setenv("SPACK_ASSETS_ROOT", t.TempDir())
	t.Setenv("SPACK_FRONTEND_STALE_ASSET_RECOVERY_ENABLE", "true")

	cfg, err := config.LoadWithOptions(config.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Frontend.StaleAssetRecovery.Enable {
		t.Fatal("expected stale asset recovery environment variable to be applied")
	}
}
