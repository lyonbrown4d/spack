package server_test

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/server"
)

func TestErrorHandlerHidesInternalError(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	app := newErrorTestApp(t, logger)
	app.Get("/boom", func(c fiber.Ctx) error {
		c.Set(server.RequestIDHeader, "rid-123")
		return errors.New("open C:\\secret\\asset.js: permission denied")
	})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/boom", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeHandlerResponseBody(t, response)

	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", response.StatusCode)
	}
	body := readResponseBody(t, response)
	if body != "Internal Server Error\nrequest_id=rid-123" {
		t.Fatalf("unexpected body %q", body)
	}
	if strings.Contains(body, "secret") || strings.Contains(body, "permission denied") {
		t.Fatalf("expected response body to hide internal error, got %q", body)
	}

	logBody := logs.String()
	if !strings.Contains(logBody, "rid-123") || !strings.Contains(logBody, "permission denied") {
		t.Fatalf("expected structured log to include request id and full error, got %q", logBody)
	}
}

func TestErrorHandlerKeepsNotFoundResponseGeneric(t *testing.T) {
	app := newErrorTestApp(t, slog.New(slog.DiscardHandler))

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/missing", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeHandlerResponseBody(t, response)

	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", response.StatusCode)
	}
	if body := readResponseBody(t, response); body != "Not found" {
		t.Fatalf("unexpected body %q", body)
	}
}

func newErrorTestApp(t *testing.T, logger *slog.Logger) *fiber.App {
	t.Helper()

	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Path = "/assets"
	cfg.Assets.Root = t.TempDir()
	cat := catalog.NewInMemoryCatalog()
	return server.NewObservedAppForTest(
		&cfg,
		logger,
		nil,
		nil,
		cat,
		assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, slog.New(slog.DiscardHandler)),
		resolver.NewResolverForTest(&cfg.Assets, cat, slog.New(slog.DiscardHandler)),
		nil,
	)
}

func readResponseBody(t *testing.T, response *http.Response) string {
	t.Helper()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func closeHandlerResponseBody(t *testing.T, response *http.Response) {
	t.Helper()

	if err := response.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}
}
