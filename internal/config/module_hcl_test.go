package config_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/lyonbrown4d/spack/internal/config"
)

type hclConfigCase struct {
	name        string
	filename    string
	httpPort    int
	assetsPath  string
	assetsRoot  string
	loggerLevel string
	httpPrefork bool
}

func assertLoadsHCLConfigCase(t *testing.T, tc hclConfigCase) {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), tc.filename)
	if err := os.WriteFile(configPath, []byte(hclConfigBody(tc)), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadWithOptions(config.LoadOptions{Files: []string{configPath}})
	if err != nil {
		t.Fatal(err)
	}
	assertLoadedHCLConfig(t, cfg, tc)
}

func hclConfigBody(tc hclConfigCase) string {
	return "" +
		"http {\n" +
		"  port = " + strconv.Itoa(tc.httpPort) + "\n" +
		"  prefork = " + strconv.FormatBool(tc.httpPrefork) + "\n" +
		"}\n" +
		"assets {\n" +
		"  path = \"" + tc.assetsPath + "\"\n" +
		"  root = \"" + tc.assetsRoot + "\"\n" +
		"}\n" +
		"logger {\n" +
		"  level = \"" + tc.loggerLevel + "\"\n" +
		"}\n"
}

func assertLoadedHCLConfig(t *testing.T, cfg *config.Config, tc hclConfigCase) {
	t.Helper()

	if cfg.HTTP.Port != tc.httpPort {
		t.Fatalf("expected http.port to be %d, got %d", tc.httpPort, cfg.HTTP.Port)
	}
	if cfg.HTTP.Prefork != tc.httpPrefork {
		t.Fatalf("expected http.prefork to be %v, got %v", tc.httpPrefork, cfg.HTTP.Prefork)
	}
	if cfg.Assets.Path != tc.assetsPath {
		t.Fatalf("expected assets.path to be %q, got %q", tc.assetsPath, cfg.Assets.Path)
	}
	if cfg.Assets.Root != tc.assetsRoot {
		t.Fatalf("expected assets.root to be %q, got %q", tc.assetsRoot, cfg.Assets.Root)
	}
	if cfg.Logger.Level != tc.loggerLevel {
		t.Fatalf("expected logger.level to be %q, got %q", tc.loggerLevel, cfg.Logger.Level)
	}
}
