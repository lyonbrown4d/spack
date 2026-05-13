package cmd

import (
	"context"
	"net/http"
	"time"

	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewConfigFlagSetForTest() *pflag.FlagSet {
	return newConfigFlagSet()
}

func ConfigLoadOptionsForTest(command *cobra.Command) config.LoadOptions {
	return configLoadOptions(command)
}

func RunHealthcheckForTest(url string, client *http.Client) error {
	return runHealthcheck(context.Background(), healthcheckOptions{
		url:     url,
		timeout: time.Second,
		client:  client,
	})
}
