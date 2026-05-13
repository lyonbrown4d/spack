package cmd_test

import (
	"io"
	"net/http"
	"testing"

	"github.com/lyonbrown4d/spack/cmd"
)

func TestHealthcheckCommandSucceedsForHealthyEndpoint(t *testing.T) {
	err := cmd.RunHealthcheckForTest("http://spack.test/livez", newHealthcheckClient(http.StatusNoContent))
	if err != nil {
		t.Fatal(err)
	}
}

func TestHealthcheckCommandFailsForUnhealthyEndpoint(t *testing.T) {
	err := cmd.RunHealthcheckForTest("http://spack.test/livez", newHealthcheckClient(http.StatusServiceUnavailable))
	if err == nil {
		t.Fatal("expected unhealthy endpoint to fail")
	}
}

func newHealthcheckClient(statusCode int) *http.Client {
	return &http.Client{
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: statusCode,
				Body:       io.NopCloser(http.NoBody),
			}, nil
		}),
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
