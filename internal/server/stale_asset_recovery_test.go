package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/server"
)

func setupStaleRecoveryRoute(
	t *testing.T,
	enable bool,
) *fiber.App {
	t.Helper()

	cfg := &config.Config{
		Frontend: config.Frontend{
			StaleAssetRecovery: config.StaleAssetRecovery{Enable: enable},
		},
	}
	rt := server.NewAssetDeliveryRuntimeForTest(cfg)

	app := fiber.New(fiber.Config{ErrorHandler: func(c fiber.Ctx, err error) error {
		return c.SendStatus(fiber.StatusNotFound)
	}})
	app.Get("/*", func(c fiber.Ctx) error {
		assetPath := c.Path()
		if assetPath != "" && assetPath[0] == '/' {
			assetPath = assetPath[1:]
		}
		if err := rt.TryStaleAssetRecovery(c, assetPath); err != nil {
			return fmt.Errorf("try stale asset recovery: %w", err)
		}
		return nil
	})
	return app
}

func TestTryStaleAssetRecoveryReturnsReloadForFingerprintedJS(t *testing.T) {
	app := setupStaleRecoveryRoute(t, true)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/assets/chunk.abc123def456.js", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeStaleRecoveryBody(t, response)

	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status 200, got %d", response.StatusCode)
	}
	if got := response.Header.Get(fiber.HeaderContentType); got != fiber.MIMEApplicationJavaScript {
		t.Fatalf("expected Content-Type %s, got %s", fiber.MIMEApplicationJavaScript, got)
	}
	if got := response.Header.Get(fiber.HeaderCacheControl); got != "no-store" {
		t.Fatalf("expected Cache-Control no-store, got %s", got)
	}
}

func TestTryStaleAssetRecoveryReturns404WhenDisabled(t *testing.T) {
	app := setupStaleRecoveryRoute(t, false)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/assets/chunk.abc123def456.js", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeStaleRecoveryBody(t, response)

	if response.StatusCode != fiber.StatusNotFound {
		t.Fatalf("expected status 404, got %d", response.StatusCode)
	}
}

func TestTryStaleAssetRecoveryReturns404ForNonJSFile(t *testing.T) {
	app := setupStaleRecoveryRoute(t, true)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/assets/style.abc123de.css", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeStaleRecoveryBody(t, response)

	if response.StatusCode != fiber.StatusNotFound {
		t.Fatalf("expected status 404 for CSS, got %d", response.StatusCode)
	}
}

func TestTryStaleAssetRecoveryReturns404ForNonFingerprintedJS(t *testing.T) {
	app := setupStaleRecoveryRoute(t, true)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/assets/app.js", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeStaleRecoveryBody(t, response)

	if response.StatusCode != fiber.StatusNotFound {
		t.Fatalf("expected status 404 for non-fingerprinted JS, got %d", response.StatusCode)
	}
}

func TestTryStaleAssetRecoveryErrorIsErrNotFound(t *testing.T) {
	app := setupStaleRecoveryRoute(t, false)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/assets/chunk.abc123def456.js", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeStaleRecoveryBody(t, response)

	if response.StatusCode != fiber.StatusNotFound {
		t.Fatalf("expected status 404, got %d", response.StatusCode)
	}
}

func closeStaleRecoveryBody(t *testing.T, response *http.Response) {
	t.Helper()

	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
}
