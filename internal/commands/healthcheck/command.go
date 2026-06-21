// Package healthcheckcmd implements the spack healthcheck command.
package healthcheckcmd

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

const (
	defaultHealthcheckURL     = "http://127.0.0.1/livez"
	defaultHealthcheckTimeout = 3 * time.Second
)

type healthcheckOptions struct {
	url     string
	timeout time.Duration
	client  *http.Client
}

func NewCommand() *cobra.Command {
	options := healthcheckOptions{
		url:     defaultHealthcheckURL,
		timeout: defaultHealthcheckTimeout,
	}

	command := &cobra.Command{
		Use:   "healthcheck",
		Short: "Check the local spack HTTP health endpoint",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHealthcheck(cmd.Context(), options)
		},
	}
	command.Flags().StringVar(&options.url, "url", options.url, "Health endpoint URL.")
	command.Flags().DurationVar(&options.timeout, "timeout", options.timeout, "Healthcheck timeout.")
	return command
}

func runHealthcheck(ctx context.Context, options healthcheckOptions) error {
	ctx, cancel := context.WithTimeout(ctx, options.timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, options.url, http.NoBody)
	if err != nil {
		return fmt.Errorf("create healthcheck request: %w", err)
	}

	response, err := healthcheckClient(options).Do(request)
	if err != nil {
		return fmt.Errorf("run healthcheck request: %w", err)
	}

	statusCode := response.StatusCode
	if err := response.Body.Close(); err != nil {
		return fmt.Errorf("close healthcheck response: %w", err)
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("healthcheck failed: status %d", statusCode)
	}

	return nil
}

func healthcheckClient(options healthcheckOptions) *http.Client {
	if options.client != nil {
		return options.client
	}
	return http.DefaultClient
}
