package server_test

import (
	"context"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func FuzzHTTPRangeRequest(f *testing.F) {
	for _, seed := range []string{
		"",
		"bytes=0-3",
		"bytes=3-",
		"bytes=-4",
		"bytes=99-100",
		"bytes=abc",
		"items=0-1",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, rangeHeader string) {
		runHTTPRangeFuzz(t, rangeHeader)
	})
}

func runHTTPRangeFuzz(t *testing.T, rangeHeader string) {
	t.Helper()
	skipWindowsRangePath(t, true)
	skipUnsupportedRangeSeed(t, rangeHeader)

	baseURL, _, _, _ := newProtocolMatrixServer(t)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/app.js", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(rangeHeader) != "" {
		request.Header.Set(fiber.HeaderRange, rangeHeader)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeHTTPBody(t, response)

	if response.StatusCode >= http.StatusInternalServerError {
		t.Fatalf("range header %q produced server error %d", rangeHeader, response.StatusCode)
	}
}

func skipUnsupportedRangeSeed(t *testing.T, rangeHeader string) {
	t.Helper()
	if len(rangeHeader) > 256 || strings.ContainsAny(rangeHeader, "\r\n\x00") {
		t.Skip("range seed is outside the HTTP header budget")
	}
}

func skipWindowsRangePath(t *testing.T, skip bool) {
	t.Helper()
	if skip && runtime.GOOS == "windows" {
		t.Skip("Fiber SendFile Range keeps file handles open under Windows test runners; Linux CI and container smoke cover this protocol path")
	}
}
