package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/source"
)

const preparedLargeFileSize = 4 * 1024 * 1024

func BenchmarkHTTPPreparedLargeFile(b *testing.B) {
	seed := []byte("spack-aot-stream\n")
	payload := bytes.Repeat(seed, preparedLargeFileSize/len(seed)+1)
	payload = payload[:preparedLargeFileSize]

	b.Run("local_stream/direct", func(b *testing.B) {
		src, file := newPreparedSendFileDirectSource(b, payload)
		benchmarkPreparedLargeFile(b, src, file, payload)
	})
	b.Run("trusted_stream/bundle", func(b *testing.B) {
		src, file := newPreparedSendFileBundleSource(b, payload)
		benchmarkPreparedLargeFile(b, src, file, payload)
	})
}

func benchmarkPreparedLargeFile(b *testing.B, src *source.LocalFS, file source.File, payload []byte) {
	b.Helper()

	response := compilePreparedSendFileResponse(b, src, file)
	runtime := &assetDeliveryRuntime{fileSources: newServerFileSourcesFromSource(src)}
	app := fiber.New()
	app.Get("/app.js", func(c fiber.Ctx) error {
		_, _, err := runtime.sendPreparedAsset(
			c,
			resolver.Request{Path: "app.js"},
			preparedSelection{response: response},
		)
		return err
	})
	b.Cleanup(func() {
		if err := app.Shutdown(); err != nil {
			b.Fatal(err)
		}
	})

	drainPreparedBenchmarkResponse(b, app)
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for b.Loop() {
		drainPreparedBenchmarkResponse(b, app)
	}
}

func drainPreparedBenchmarkResponse(b *testing.B, app *fiber.App) {
	b.Helper()

	request := httptest.NewRequestWithContext(b.Context(), http.MethodGet, "/app.js", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		b.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		b.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		b.Fatal(err)
	}
}
