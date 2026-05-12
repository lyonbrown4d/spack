package cmd

import (
	"testing"

	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/spf13/cobra"
)

func TestConfigLoadOptionsUsesParsedCommandFlags(t *testing.T) {
	command := &cobra.Command{Use: "spack-test"}
	command.Flags().AddFlagSet(newConfigFlagSet())
	if err := command.ParseFlags([]string{
		"--assets.root=/tmp/spack-assets",
		"--http.port=18080",
		"--debug.enable=false",
	}); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.LoadWithOptions(configLoadOptions(command))
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Assets.Root != "/tmp/spack-assets" {
		t.Fatalf("expected parsed assets.root flag, got %q", loaded.Assets.Root)
	}
	if loaded.HTTP.Port != 18080 {
		t.Fatalf("expected parsed http.port flag, got %d", loaded.HTTP.Port)
	}
	if loaded.Debug.Enable {
		t.Fatal("expected parsed debug.enable=false flag")
	}
}
