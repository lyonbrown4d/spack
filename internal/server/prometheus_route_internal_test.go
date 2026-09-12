package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dix"
	obsprom "github.com/arcgolabs/observabilityx/prometheus"
	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	clientprom "github.com/prometheus/client_golang/prometheus"
)

func TestPrometheusRoutePrecedesAssetFallback(t *testing.T) {
	app := newPrometheusRouteTestApp()
	response := testPrometheusRoute(t, app, "/prometheus")
	defer closePrometheusResponse(t, response)

	body := readPrometheusResponse(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected metrics status 200, got %d", response.StatusCode)
	}
	if contentType := response.Header.Get(fiber.HeaderContentType); strings.HasPrefix(contentType, fiber.MIMETextHTML) {
		t.Fatalf("expected Prometheus content type, got %q", contentType)
	}
	if strings.Contains(body, "asset fallback") {
		t.Fatalf("expected Prometheus route to precede asset fallback, got %q", body)
	}
}

func TestPrometheusRouteUsesStrictPathMatching(t *testing.T) {
	app := newPrometheusRouteTestApp()
	response := testPrometheusRoute(t, app, "/prometheus/")
	defer closePrometheusResponse(t, response)

	body := readPrometheusResponse(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected strict route fallback status 200, got %d", response.StatusCode)
	}
	if contentType := response.Header.Get(fiber.HeaderContentType); !strings.HasPrefix(contentType, fiber.MIMETextHTML) {
		t.Fatalf("expected strict route fallback HTML content type, got %q", contentType)
	}
	if !strings.Contains(body, "asset fallback") {
		t.Fatalf("expected /prometheus/ to use asset fallback, got %q", body)
	}
}

func newPrometheusRouteTestApp() *fiber.App {
	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	registry := clientprom.NewRegistry()
	adapter := obsprom.New(
		obsprom.WithRegisterer(registry),
		obsprom.WithGatherer(registry),
	)
	diagnostics := newDiagnosticsRoutesRegistration(newDiagnosticsRoutesRuntime(
		&cfg,
		slog.New(slog.DiscardHandler),
		adapter,
		catalog.NewInMemoryCatalog(),
	))
	fallback := newAppRegistration(300, "asset_fallback", func(app *fiber.App) {
		app.Get("/*", func(c fiber.Ctx) error {
			return c.Type("html").SendString("asset fallback")
		})
	})
	return newServerFromDeps(
		&cfg,
		dix.AppMeta{Version: "test"},
		slog.New(slog.DiscardHandler),
		cxlist.NewList(diagnostics, fallback),
	)
}

func testPrometheusRoute(t *testing.T, app *fiber.App, path string) *http.Response {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func readPrometheusResponse(t *testing.T, response *http.Response) string {
	t.Helper()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func closePrometheusResponse(t *testing.T, response *http.Response) {
	t.Helper()

	if err := response.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}
}
