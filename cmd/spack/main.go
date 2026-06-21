// Package main starts the spack CLI application.
package main

import (
	_ "github.com/joho/godotenv/autoload"
	"github.com/spf13/cobra"
)

func main() {
	cobra.CheckErr(execute())
}
