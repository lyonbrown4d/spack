package server_test

import (
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func FuzzHTTPRangeRequest(f *testing.F) {
	for _, seed := range []string{
		"",
		"bytes=0-3",
		"Bytes=3-",
		"bytes=-4",
		"bytes=-0",
		"bytes=4-3",
		"bytes=99-100",
		"bytes=abc",
		"bytes=0-1,2-3",
		"items=0-1",
		"pages=1-3=note",
		"bytes=9223372036854775808-",
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
	skipInvalidRangeFuzzSeed(t, rangeHeader)

	response := requestHTTPRangeFuzz(t, rangeHeader)
	defer closeHTTPBody(t, response)

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	assertHTTPRangeFuzzResponse(t, rangeHeader, response, body)
}

func requestHTTPRangeFuzz(t *testing.T, rangeHeader string) *http.Response {
	t.Helper()
	baseURL, _, _, _ := newProtocolMatrixServer(t)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/app.js", http.NoBody)
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
	return response
}

func assertHTTPRangeFuzzResponse(t *testing.T, rangeHeader string, response *http.Response, body []byte) {
	t.Helper()
	switch response.StatusCode {
	case http.StatusOK:
		assertHTTPRangeFuzzOK(t, rangeHeader, response, body)
	case http.StatusPartialContent:
		assertHTTPRangeFuzzPartial(t, rangeHeader, response, body)
	case http.StatusRequestedRangeNotSatisfiable:
		assertHTTPRangeFuzzUnsatisfiable(t, rangeHeader, response, body)
	default:
		t.Fatalf("range header %q produced unexpected status %d", rangeHeader, response.StatusCode)
	}
}

func assertHTTPRangeFuzzOK(t *testing.T, rangeHeader string, response *http.Response, body []byte) {
	t.Helper()
	if string(body) != "0123456789" {
		t.Fatalf("range header %q produced incomplete 200 body %q", rangeHeader, string(body))
	}
	if contentRange := response.Header.Get(fiber.HeaderContentRange); contentRange != "" {
		t.Fatalf("range header %q produced Content-Range on 200: %q", rangeHeader, contentRange)
	}
}

func assertHTTPRangeFuzzPartial(t *testing.T, rangeHeader string, response *http.Response, body []byte) {
	t.Helper()
	if len(body) == 0 || len(body) > len("0123456789") {
		t.Fatalf("range header %q produced invalid 206 body length %d", rangeHeader, len(body))
	}
	if contentRange := response.Header.Get(fiber.HeaderContentRange); !strings.HasPrefix(contentRange, "bytes ") {
		t.Fatalf("range header %q produced invalid 206 Content-Range %q", rangeHeader, contentRange)
	}
	if contentLength := response.Header.Get(fiber.HeaderContentLength); contentLength != strconv.Itoa(len(body)) {
		t.Fatalf("range header %q produced Content-Length %q for body length %d", rangeHeader, contentLength, len(body))
	}
}

func assertHTTPRangeFuzzUnsatisfiable(t *testing.T, rangeHeader string, response *http.Response, body []byte) {
	t.Helper()
	if len(body) != 0 {
		t.Fatalf("range header %q produced non-empty 416 body %q", rangeHeader, string(body))
	}
	if contentRange := response.Header.Get(fiber.HeaderContentRange); contentRange != "bytes */10" {
		t.Fatalf("range header %q produced invalid 416 Content-Range %q", rangeHeader, contentRange)
	}
	if contentLength := response.Header.Get(fiber.HeaderContentLength); contentLength != "0" {
		t.Fatalf("range header %q produced invalid 416 Content-Length %q", rangeHeader, contentLength)
	}
}
func skipInvalidRangeFuzzSeed(t *testing.T, rangeHeader string) {
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
