// Package main starts the spack CLI application.
package main

import (
	_ "github.com/joho/godotenv/autoload"
	"github.com/lyonbrown4d/spack/cmd"
	"github.com/spf13/cobra"
)

func main() {
	cobra.CheckErr(cmd.Execute())
}
