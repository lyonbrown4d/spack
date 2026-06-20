// Package main starts the SPACK compiler binary.
package main

import (
	_ "github.com/joho/godotenv/autoload"
	"github.com/lyonbrown4d/spack/cmd"
	"github.com/spf13/cobra"
)

func main() {
	cobra.CheckErr(cmd.ExecuteCompiler())
}
