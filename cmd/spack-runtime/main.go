// Package main starts the SPACK runtime binary.
package main

import (
	"github.com/spf13/cobra"
)

func main() {
	cobra.CheckErr(execute())
}
