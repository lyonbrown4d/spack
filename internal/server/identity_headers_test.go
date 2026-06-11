package server_test

import (
	"net/http"
	"testing"

	"github.com/arcgolabs/dix"
	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/constant"
	"github.com/lyonbrown4d/spack/internal/server"
)

func TestServerHeaderHiddenByDefault(t *testing.T) {
	cfg := config.DefaultConfigForTest()

	header := server.ServerHeaderForTest(&cfg, dix.AppMeta{Version: "1.2.3"})
	if header != "" {
		t.Fatalf("expected empty server header by default, got %q", header)
	}
}

func TestServerHeaderCanBeExposed(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	cfg.HTTP.ExposeServerHeader = true

	header := server.ServerHeaderForTest(&cfg, dix.AppMeta{Version: "1.2.3"})
	if header != constant.ServerHeaderPrefix {
		t.Fatalf("expected non-versioned server header by default, got %q", header)
	}
}

func TestServerHeaderCanBeVersioned(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	cfg.HTTP.ExposeServerHeader = true
	cfg.HTTP.ExposeServerVersion = true

	header := server.ServerHeaderForTest(&cfg, dix.AppMeta{Version: "1.2.3"})
	if header != constant.ServerHeaderPrefix+"/1.2.3" {
		t.Fatalf("expected versioned server header, got %q", header)
	}
}

func TestIdentityHeaderMiddlewareStripsIdentityHeadersByDefault(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	response := identityHeaderResponse(t, &cfg)
	defer closeResponseBody(t, response)

	if got := response.Header.Get(fiber.HeaderServer); got != "" {
		t.Fatalf("expected Server header to be stripped, got %q", got)
	}
	if got := response.Header.Get("X-Powered-By"); got != "" {
		t.Fatalf("expected X-Powered-By header to be stripped, got %q", got)
	}
}

func TestIdentityHeaderMiddlewareKeepsServerHeaderWhenExposed(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	cfg.HTTP.ExposeServerHeader = true
	response := identityHeaderResponse(t, &cfg)
	defer closeResponseBody(t, response)

	if got := response.Header.Get(fiber.HeaderServer); got != "visible-server" {
		t.Fatalf("expected Server header to remain, got %q", got)
	}
	if got := response.Header.Get("X-Powered-By"); got != "" {
		t.Fatalf("expected X-Powered-By header to be stripped, got %q", got)
	}
}

func identityHeaderResponse(t *testing.T, cfg *config.Config) *http.Response {
	t.Helper()
	app := fiber.New()
	app.Use(server.IdentityHeaderMiddlewareForTest(cfg))
	app.Get("/", func(c fiber.Ctx) error {
		c.Set(fiber.HeaderServer, "visible-server")
		c.Set("X-Powered-By", "visible-framework")
		return c.SendStatus(fiber.StatusNoContent)
	})

	response, err := app.Test(httptestRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func closeResponseBody(t *testing.T, response *http.Response) {
	t.Helper()
	if response == nil || response.Body == nil {
		return
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
}

func httptestRequest(t *testing.T) *http.Request {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	return request
}
