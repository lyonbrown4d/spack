// Package main starts the SPACK compiler binary.
package main

import (
	_ "github.com/joho/godotenv/autoload"
	"github.com/spf13/cobra"
)

func main() {
	cobra.CheckErr(execute())
}
