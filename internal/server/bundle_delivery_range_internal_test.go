package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/resolver"
)

type rangeTestHeaderPlan struct{}

func (rangeTestHeaderPlan) ApplySendFileOverrides(fiber.Ctx, bool) {}

func TestSendBundleBodyRangeProtocol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		body              string
		rangeHeader       string
		wantStatus        int
		wantBody          string
		wantContentRange  string
		wantContentLength string
	}{
		{name: "first byte", body: "0123456789", rangeHeader: "bytes=0-0", wantStatus: http.StatusPartialContent, wantBody: "0", wantContentRange: "bytes 0-0/10", wantContentLength: "1"},
		{name: "case insensitive unit with whitespace", body: "0123456789", rangeHeader: "Bytes = 2-5", wantStatus: http.StatusPartialContent, wantBody: "2345", wantContentRange: "bytes 2-5/10", wantContentLength: "4"},
		{name: "open ended range", body: "0123456789", rangeHeader: "bytes=7-", wantStatus: http.StatusPartialContent, wantBody: "789", wantContentRange: "bytes 7-9/10", wantContentLength: "3"},
		{name: "suffix range", body: "0123456789", rangeHeader: "bytes=-3", wantStatus: http.StatusPartialContent, wantBody: "789", wantContentRange: "bytes 7-9/10", wantContentLength: "3"},
		{name: "end beyond representation is clamped", body: "0123456789", rangeHeader: "bytes=8-99", wantStatus: http.StatusPartialContent, wantBody: "89", wantContentRange: "bytes 8-9/10", wantContentLength: "2"},
		{name: "suffix larger than representation", body: "0123456789", rangeHeader: "bytes=-99", wantStatus: http.StatusPartialContent, wantBody: "0123456789", wantContentRange: "bytes 0-9/10", wantContentLength: "10"},
		{name: "unsupported unit is ignored", body: "0123456789", rangeHeader: "items=0-1", wantStatus: http.StatusOK, wantBody: "0123456789", wantContentLength: "10"},
		{name: "unsupported unit can contain equals", body: "0123456789", rangeHeader: "pages=1-3=note", wantStatus: http.StatusOK, wantBody: "0123456789", wantContentLength: "10"},
		{name: "missing equals is malformed", body: "0123456789", rangeHeader: "bytes", wantStatus: http.StatusRequestedRangeNotSatisfiable, wantContentRange: "bytes */10", wantContentLength: "0"},
		{name: "empty range set is malformed", body: "0123456789", rangeHeader: "bytes=", wantStatus: http.StatusRequestedRangeNotSatisfiable, wantContentRange: "bytes */10", wantContentLength: "0"},
		{name: "missing bounds is malformed", body: "0123456789", rangeHeader: "bytes=-", wantStatus: http.StatusRequestedRangeNotSatisfiable, wantContentRange: "bytes */10", wantContentLength: "0"},
		{name: "descending bounds are malformed", body: "0123456789", rangeHeader: "bytes=4-3", wantStatus: http.StatusRequestedRangeNotSatisfiable, wantContentRange: "bytes */10", wantContentLength: "0"},
		{name: "non numeric bound is malformed", body: "0123456789", rangeHeader: "bytes=a-1", wantStatus: http.StatusRequestedRangeNotSatisfiable, wantContentRange: "bytes */10", wantContentLength: "0"},
		{name: "extra equals is malformed", body: "0123456789", rangeHeader: "bytes=0-1=tag", wantStatus: http.StatusRequestedRangeNotSatisfiable, wantContentRange: "bytes */10", wantContentLength: "0"},
		{name: "start at representation size is unsatisfiable", body: "0123456789", rangeHeader: "bytes=10-", wantStatus: http.StatusRequestedRangeNotSatisfiable, wantContentRange: "bytes */10", wantContentLength: "0"},
		{name: "zero suffix is unsatisfiable", body: "0123456789", rangeHeader: "bytes=-0", wantStatus: http.StatusRequestedRangeNotSatisfiable, wantContentRange: "bytes */10", wantContentLength: "0"},
		{name: "range on empty representation is unsatisfiable", rangeHeader: "bytes=0-0", wantStatus: http.StatusRequestedRangeNotSatisfiable, wantContentRange: "bytes */0", wantContentLength: "0"},
		{name: "multiple satisfiable ranges are rejected", body: "0123456789", rangeHeader: "bytes=0-1,2-3", wantStatus: http.StatusRequestedRangeNotSatisfiable, wantContentRange: "bytes */10", wantContentLength: "0"},
		{name: "satisfiable and unsatisfiable ranges are rejected", body: "0123456789", rangeHeader: "bytes=0-1,20-30", wantStatus: http.StatusRequestedRangeNotSatisfiable, wantContentRange: "bytes */10", wantContentLength: "0"},
		{name: "empty list element is rejected as multiple range syntax", body: "0123456789", rangeHeader: "bytes=,0-1", wantStatus: http.StatusRequestedRangeNotSatisfiable, wantContentRange: "bytes */10", wantContentLength: "0"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assertBundleRangeResponse(t, testCase.body, testCase.rangeHeader, testCase.wantStatus, testCase.wantBody, testCase.wantContentRange, testCase.wantContentLength)
		})
	}
}

func assertBundleRangeResponse(
	t *testing.T,
	body string,
	rangeHeader string,
	wantStatus int,
	wantBody string,
	wantContentRange string,
	wantContentLength string,
) {
	t.Helper()

	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		request := resolver.Request{RangeRequested: c.Get(fiber.HeaderRange) != ""}
		_, err := sendBundleBody(c, request, []byte(body), rangeTestHeaderPlan{})
		return err
	})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	request.Header.Set(fiber.HeaderRange, rangeHeader)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("expected status %d, got %d", wantStatus, response.StatusCode)
	}
	if string(responseBody) != wantBody {
		t.Fatalf("expected body %q, got %q", wantBody, string(responseBody))
	}
	if contentRange := response.Header.Get(fiber.HeaderContentRange); contentRange != wantContentRange {
		t.Fatalf("expected Content-Range %q, got %q", wantContentRange, contentRange)
	}
	if contentLength := response.Header.Get(fiber.HeaderContentLength); contentLength != wantContentLength {
		t.Fatalf("expected Content-Length %q, got %q", wantContentLength, contentLength)
	}
}
