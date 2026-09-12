package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/server"
)

func TestMetricsMiddlewareRecordsAssetDeliveryBytes(t *testing.T) {
	obs := &recordingObservability{}
	app := fiber.New()
	app.Use(server.MetricsMiddlewareForTest(obs))
	app.Get("/asset", func(c fiber.Ctx) error {
		server.SetAssetDeliveryForTest(c, "prepared_memory")
		return c.SendString("asset")
	})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/asset", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeBody(t, response)

	metric := findMetric(t, obs.counters, "http_asset_delivery_bytes_total")
	assertMetricContract(t, metric, map[string]any{
		"method":   http.MethodGet,
		"route":    "/asset",
		"status":   "200",
		"delivery": "prepared_memory",
	})
	if metric.value != int64(len("asset")) {
		t.Fatalf("expected delivered byte count %d, got %v", len("asset"), metric.value)
	}
}
