// Package cmdkit provides CLI command helpers shared by SPACK binaries.
package cmdkit

import (
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

func Execute(command *cobra.Command) error {
	if err := command.Execute(); err != nil {
		return oops.In("command").Wrap(oops.Wrapf(err, "execute root command"))
	}
	return nil
}
