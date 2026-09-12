package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/source"
)

const preparedSendFileBody = "trusted bundle body"

type preparedSendFileTrustCase struct {
	name        string
	src         *source.LocalFS
	file        source.File
	wantTrusted bool
}

func TestPreparedCompilerBindsSendFileOnlyForTrustedBundle(t *testing.T) {
	bundleSource, bundleFile := newPreparedSendFileBundleSource(t, []byte(preparedSendFileBody))
	directSource, directFile := newPreparedSendFileDirectSource(t, []byte(preparedSendFileBody))

	tests := []preparedSendFileTrustCase{
		{name: "bundle", src: bundleSource, file: bundleFile, wantTrusted: true},
		{name: "direct", src: directSource, file: directFile, wantTrusted: false},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			response := compilePreparedSendFileResponse(t, testCase.src, testCase.file)
			if got := response.sendFile != nil; got != testCase.wantTrusted {
				t.Fatalf("expected trusted send file %t, got %t", testCase.wantTrusted, got)
			}
		})
	}
}

type preparedProtocolCase struct {
	name       string
	method     string
	headers    map[string]string
	wantStatus int
	wantBody   string
}

func TestPreparedTrustedStreamPreservesProtocolHeaders(t *testing.T) {
	modTime := time.Date(2026, time.September, 12, 8, 30, 0, 0, time.UTC)
	response := newPreparedMapFSResponse(t, preparedSendFileBody, modTime)
	app := newPreparedResponseApp(response, nil)

	tests := []preparedProtocolCase{
		{name: "get", method: http.MethodGet, wantStatus: http.StatusOK, wantBody: preparedSendFileBody},
		{name: "head", method: http.MethodHead, wantStatus: http.StatusOK},
		{
			name:       "etag not modified",
			method:     http.MethodGet,
			headers:    map[string]string{fiber.HeaderIfNoneMatch: "\"trusted-etag\""},
			wantStatus: http.StatusNotModified,
		},
		{
			name:       "modified since",
			method:     http.MethodGet,
			headers:    map[string]string{fiber.HeaderIfModifiedSince: modTime.Format(http.TimeFormat)},
			wantStatus: http.StatusNotModified,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			assertPreparedProtocolResponse(t, app, testCase, modTime)
		})
	}
}

func assertPreparedProtocolResponse(
	t *testing.T,
	app testFiberApp,
	testCase preparedProtocolCase,
	modTime time.Time,
) {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), testCase.method, "/app.js", http.NoBody)
	for key, value := range testCase.headers {
		request.Header.Set(key, value)
	}
	result := sendPreparedTestRequest(t, app, request)
	defer closePreparedResponse(t, result)

	body := readPreparedResponseBody(t, result)
	if result.StatusCode != testCase.wantStatus {
		t.Fatalf("expected status %d, got %d", testCase.wantStatus, result.StatusCode)
	}
	if body != testCase.wantBody {
		t.Fatalf("expected body %q, got %q", testCase.wantBody, body)
	}
	assertPreparedSendFileHeaders(t, result, modTime)
}

func TestPreparedTrustedStreamRangeUsesGuardedStream(t *testing.T) {
	directSource, directFile := newPreparedSendFileDirectSource(t, []byte("guarded stream body"))
	response := newPreparedMapFSResponse(t, "untrusted map body", time.Time{})
	response.result.FilePath = directFile.FullPath
	response.result.Asset.FullPath = directFile.FullPath
	runtime := &assetDeliveryRuntime{fileSources: newServerFileSourcesFromSource(directSource)}
	app := newPreparedResponseApp(response, runtime)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/app.js", http.NoBody)
	request.Header.Set(fiber.HeaderRange, "bytes=0-6")
	result := sendPreparedTestRequest(t, app, request)
	defer closePreparedResponse(t, result)

	body := readPreparedResponseBody(t, result)
	if result.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d", result.StatusCode)
	}
	if body != "guarded" {
		t.Fatalf("expected guarded range body, got %q", body)
	}
}
func TestPreparedTrustedStreamMissingVariantFallsBackBeforeResponseWrite(t *testing.T) {
	fixture := newMissingPreparedVariantFixture(t)
	fixture.selection.response.sendFile = &preparedSendFile{
		root: fstest.MapFS{},
		path: "app.js.br",
	}

	runtime := &assetDeliveryRuntime{
		prepared:    fixture.service,
		fileSources: fixture.service.fileSources,
	}
	app := fiber.New()
	app.Get("/app.js", func(c fiber.Ctx) error {
		_, _, err := runtime.sendPreparedAsset(c, fixture.request, fixture.selection)
		return err
	})

	result := sendPreparedTestRequest(
		t,
		app,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/app.js", http.NoBody),
	)
	assertMissingPreparedVariantFallback(t, result)
	if err := result.Body.Close(); err != nil {
		t.Fatal(err)
	}
}

type missingPreparedVariantFixture struct {
	service   *PreparedService
	request   resolver.Request
	selection preparedSelection
}

func newMissingPreparedVariantFixture(t *testing.T) missingPreparedVariantFixture {
	t.Helper()

	const identityBody = "identity body"
	root := t.TempDir()
	identityPath := filepath.Join(root, "app.js")
	writePreparedSendFileFixture(t, identityPath, identityBody)
	missingVariantPath := filepath.Join(root, "app.js.br")

	cfg := config.DefaultConfigForTest()
	cfg.Assets.Root = root
	cfg.Compression.CacheDir = ""
	cfg.HTTP.MemoryCache.Enable = false
	cat := catalog.NewInMemoryCatalog()
	upsertMissingPreparedVariantFixture(t, cat, identityPath, missingVariantPath, identityBody)

	service := newPreparedService(
		&cfg,
		cat,
		slog.New(slog.DiscardHandler),
		nil,
		nil,
		nil,
	)
	t.Cleanup(func() {
		if err := service.stop(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	if err := service.Rebuild(t.Context()); err != nil {
		t.Fatal(err)
	}

	request := resolver.Request{Path: "app.js", AcceptEncoding: "br"}
	selection, ok := service.Resolve(newPreparedRequest(request, "")).Get()
	if !ok || selection.response == nil || selection.response.variant() == nil {
		t.Fatal("expected prepared encoding variant")
	}
	return missingPreparedVariantFixture{
		service:   service,
		request:   request,
		selection: selection,
	}
}

func writePreparedSendFileFixture(t *testing.T, path, body string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func upsertMissingPreparedVariantFixture(
	t *testing.T,
	cat catalog.Catalog,
	identityPath string,
	variantPath string,
	identityBody string,
) {
	t.Helper()

	if err := cat.UpsertAsset(&catalog.Asset{
		Path:       "app.js",
		FullPath:   identityPath,
		Size:       int64(len(identityBody)),
		MediaType:  "application/javascript; charset=utf-8",
		SourceHash: "identity-hash",
		ETag:       "\"identity-etag\"",
	}); err != nil {
		t.Fatal(err)
	}
	if err := cat.UpsertVariant(&catalog.Variant{
		ID:           "app.js|encoding=br",
		AssetPath:    "app.js",
		ArtifactPath: variantPath,
		Size:         7,
		MediaType:    "application/javascript; charset=utf-8",
		SourceHash:   "identity-hash",
		ETag:         "\"variant-etag\"",
		Encoding:     "br",
	}); err != nil {
		t.Fatal(err)
	}
}

func assertMissingPreparedVariantFallback(t *testing.T, result *http.Response) {
	t.Helper()

	const identityBody = "identity body"
	body := readPreparedResponseBody(t, result)
	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected fallback status 200, got %d", result.StatusCode)
	}
	if body != identityBody {
		t.Fatalf("expected fallback body %q, got %q", identityBody, body)
	}
	if got := result.Header.Get(fiber.HeaderETag); got != "\"identity-etag\"" {
		t.Fatalf("expected identity etag, got %q", got)
	}
	if got := result.Header.Get(fiber.HeaderContentEncoding); got != "" {
		t.Fatalf("expected cleared content-encoding, got %q", got)
	}
	if got := result.Header.Get(fiber.HeaderContentType); got != "application/javascript; charset=utf-8" {
		t.Fatalf("expected identity content type, got %q", got)
	}
}

func TestPreparedTrustedStreamReleasesBundleOnShutdown(t *testing.T) {
	sourceFS, file := newPreparedSendFileBundleSource(t, []byte(preparedSendFileBody))
	response := compilePreparedSendFileResponse(t, sourceFS, file)
	app := newPreparedResponseApp(response, nil)

	result := sendPreparedTestRequest(
		t,
		app,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/app.js", http.NoBody),
	)
	if body := readPreparedResponseBody(t, result); body != preparedSendFileBody {
		t.Fatalf("expected body %q, got %q", preparedSendFileBody, body)
	}
	if err := result.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if err := app.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if err := sourceFS.Cleanup(); err != nil {
		t.Fatalf("cleanup bundle source after request shutdown: %v", err)
	}
	if err := os.RemoveAll(sourceFS.Root()); err != nil {
		t.Fatalf("remove bundle root after request shutdown: %v", err)
	}
}
