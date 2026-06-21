package cmdkit_test

import (
	"testing"

	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/configschema"
	"github.com/spf13/cobra"
)

func TestNewConfigFlagSetRegistersSchemaFlags(t *testing.T) {
	defaults := config.DefaultConfig()
	flags := cmdkit.NewConfigFlagSet()

	for _, schemaFlag := range configschema.Flags() {
		flag := flags.Lookup(schemaFlag.Name)
		if flag == nil {
			t.Fatalf("expected config flag %q to be registered", schemaFlag.Name)
		}
		if got := flag.Value.Type(); got != string(schemaFlag.Kind) {
			t.Fatalf("expected config flag %q type %q, got %q", schemaFlag.Name, schemaFlag.Kind, got)
		}
		if want := schemaFlag.DefaultString(defaults); flag.DefValue != want {
			t.Fatalf("expected config flag %q default %q, got %q", schemaFlag.Name, want, flag.DefValue)
		}
		if flag.Usage != schemaFlag.Usage {
			t.Fatalf("expected config flag %q usage %q, got %q", schemaFlag.Name, schemaFlag.Usage, flag.Usage)
		}
	}
}

func TestConfigLoadOptionsUsesParsedCommandFlags(t *testing.T) {
	command := &cobra.Command{Use: "spack-test"}
	command.Flags().AddFlagSet(cmdkit.NewConfigFlagSet())
	if err := command.ParseFlags([]string{
		"--assets.root=/tmp/spack-assets",
		"--http.port=18080",
		"--http.expose_server_header=true",
		"--http.expose_server_version=true",
		"--debug.enable=false",
		"--metrics.enable=false",
		"--image.max_source_bytes=2048",
		"--image.max_source_pixels=4096",
		"--image.max_output_variants=3",
		"--image.min_saving_ratio=0.2",
	}); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.LoadWithOptions(cmdkit.ConfigLoadOptions(command))
	if err != nil {
		t.Fatal(err)
	}

	assertParsedCommandConfig(t, loaded)
}

func assertParsedCommandConfig(t *testing.T, loaded *config.Config) {
	t.Helper()
	assertParsedCoreCommandConfig(t, loaded)
	assertParsedImageCommandConfig(t, loaded.Image)
}

func assertParsedCoreCommandConfig(t *testing.T, loaded *config.Config) {
	t.Helper()

	if loaded.Assets.Root != "/tmp/spack-assets" {
		t.Fatalf("expected parsed assets.root flag, got %q", loaded.Assets.Root)
	}
	if loaded.HTTP.Port != 18080 {
		t.Fatalf("expected parsed http.port flag, got %d", loaded.HTTP.Port)
	}
	if !loaded.HTTP.ExposeServerHeader {
		t.Fatal("expected parsed http.expose_server_header=true flag")
	}
	if !loaded.HTTP.ExposeServerVersion {
		t.Fatal("expected parsed http.expose_server_version=true flag")
	}
	if loaded.Debug.Enable {
		t.Fatal("expected parsed debug.enable=false flag")
	}
	if loaded.Metrics.Enable {
		t.Fatal("expected parsed metrics.enable=false flag")
	}
}

func assertParsedImageCommandConfig(t *testing.T, image config.Image) {
	t.Helper()

	if image.MaxSourceBytes != 2048 {
		t.Fatalf("expected parsed image.max_source_bytes=2048, got %d", image.MaxSourceBytes)
	}
	if image.MaxSourcePixels != 4096 {
		t.Fatalf("expected parsed image.max_source_pixels=4096, got %d", image.MaxSourcePixels)
	}
	if image.MaxOutputVariants != 3 {
		t.Fatalf("expected parsed image.max_output_variants=3, got %d", image.MaxOutputVariants)
	}
	if image.MinSavingRatio != 0.2 {
		t.Fatalf("expected parsed image.min_saving_ratio=0.2, got %f", image.MinSavingRatio)
	}
}
