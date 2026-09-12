package cmdkit_test

import (
	"testing"

	"github.com/arcgolabs/configx"
	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestNewConfigFlagSetRegistersSchemaFlags(t *testing.T) {
	defaults := config.DefaultConfig()
	schema, err := configx.SchemaOf(defaults)
	if err != nil {
		t.Fatal(err)
	}
	flags, err := cmdkit.NewConfigFlagSet()
	if err != nil {
		t.Fatal(err)
	}

	for _, field := range schema.Fields() {
		assertSchemaFieldFlag(t, flags, field)
	}
}

func assertSchemaFieldFlag(t *testing.T, flags *pflag.FlagSet, field configx.SchemaField) {
	t.Helper()
	flag := flags.Lookup(field.FlagName)
	if !field.CLIEnabled {
		if flag != nil {
			t.Fatalf("expected config field %q to remain CLI-disabled", field.Path)
		}
		return
	}
	if flag == nil {
		t.Fatalf("expected config flag %q to be registered", field.FlagName)
	}
	if field.Usage == "" {
		t.Fatalf("expected config flag %q to have usage metadata", field.FlagName)
	}
	if got := flag.Value.Type(); got != string(field.Kind) {
		t.Fatalf("expected config flag %q type %q, got %q", field.FlagName, field.Kind, got)
	}
	wantDefault := field.Default
	if field.Kind == configx.SchemaKindStringSlice {
		wantDefault = "[" + wantDefault + "]"
	}
	if flag.DefValue != wantDefault {
		t.Fatalf("expected config flag %q default %q, got %q", field.FlagName, wantDefault, flag.DefValue)
	}
	if flag.Usage != field.Usage {
		t.Fatalf("expected config flag %q usage %q, got %q", field.FlagName, field.Usage, flag.Usage)
	}
}

func TestCloneVisitedConfigFlagsPreservesStringSlices(t *testing.T) {
	source, err := cmdkit.NewConfigFlagSet()
	if err != nil {
		t.Fatal(err)
	}

	if setErr := source.Set("assets.include", "**/*.js,assets/**"); setErr != nil {
		t.Fatal(setErr)
	}

	cloned, err := cmdkit.CloneVisitedConfigFlags(source)
	if err != nil {
		t.Fatal(err)
	}
	values, err := cloned.GetStringSlice("assets.include")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0] != "**/*.js" || values[1] != "assets/**" {
		t.Fatalf("unexpected cloned assets.include values: %v", values)
	}
}

func TestConfigLoadOptionsUsesParsedCommandFlags(t *testing.T) {
	command := &cobra.Command{Use: "spack-test"}
	flags, err := cmdkit.NewConfigFlagSet()
	if err != nil {
		t.Fatal(err)
	}
	command.Flags().AddFlagSet(flags)
	if parseErr := command.ParseFlags([]string{
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
	}); parseErr != nil {
		t.Fatal(parseErr)
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
