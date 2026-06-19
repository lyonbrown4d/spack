package server_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
)

type protocolMatrixCase struct {
	name              string
	method            string
	headers           map[string]string
	wantStatus        int
	wantBody          string
	wantETag          string
	wantEncoding      string
	wantContentRange  string
	wantContentLength string
	skipWindows       bool
}

func TestAssetHTTPProtocolMatrix(t *testing.T) {
	baseURL, etag, variantETag, lastModified := newProtocolMatrixServer(t)

	for _, tc := range protocolMatrixCases(etag, variantETag, lastModified) {
		t.Run(tc.name, func(t *testing.T) {
			runProtocolMatrixCase(t, baseURL, lastModified, tc)
		})
	}
}

func protocolMatrixCases(etag, variantETag, lastModified string) []protocolMatrixCase {
	return []protocolMatrixCase{
		{
			name:              "get origin",
			method:            http.MethodGet,
			wantStatus:        http.StatusOK,
			wantBody:          "0123456789",
			wantETag:          etag,
			wantContentLength: "10",
		},
		{
			name:              "head origin",
			method:            http.MethodHead,
			wantStatus:        http.StatusOK,
			wantETag:          etag,
			wantContentLength: "10",
		},
		{
			name:       "if none match",
			method:     http.MethodGet,
			headers:    map[string]string{fiber.HeaderIfNoneMatch: etag},
			wantStatus: http.StatusNotModified,
			wantETag:   etag,
		},
		{
			name:       "if modified since",
			method:     http.MethodGet,
			headers:    map[string]string{fiber.HeaderIfModifiedSince: lastModified},
			wantStatus: http.StatusNotModified,
			wantETag:   etag,
		},
		{
			name:         "accept encoding variant",
			method:       http.MethodGet,
			headers:      map[string]string{fiber.HeaderAcceptEncoding: "br"},
			wantStatus:   http.StatusOK,
			wantBody:     "br",
			wantETag:     variantETag,
			wantEncoding: "br",
		},
		{
			name:              "range bypasses encoded variants",
			method:            http.MethodGet,
			headers:           map[string]string{fiber.HeaderAcceptEncoding: "br", fiber.HeaderRange: "bytes=2-5"},
			wantStatus:        http.StatusPartialContent,
			wantBody:          "2345",
			wantETag:          etag,
			wantContentRange:  "bytes 2-5/10",
			wantContentLength: "4",
			skipWindows:       true,
		},
	}
}

func runProtocolMatrixCase(t *testing.T, baseURL, lastModified string, tc protocolMatrixCase) {
	t.Helper()
	skipWindowsRangePath(t, tc.skipWindows)

	request, err := http.NewRequestWithContext(context.Background(), tc.method, baseURL+"/app.js", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range tc.headers {
		request.Header.Set(key, value)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeHTTPBody(t, response)

	assertProtocolMatrixResponse(t, response, lastModified, tc)
	assertProtocolMatrixBody(t, response, tc.wantBody)
}

func assertProtocolMatrixResponse(t *testing.T, response *http.Response, lastModified string, tc protocolMatrixCase) {
	t.Helper()

	assertStatus(t, response.StatusCode, tc.wantStatus)
	assertOptionalHeader(t, response, fiber.HeaderETag, tc.wantETag)
	assertHeaderEquals(t, fiber.HeaderLastModified, response.Header.Get(fiber.HeaderLastModified), lastModified)
	assertHeaderEquals(t, fiber.HeaderContentEncoding, response.Header.Get(fiber.HeaderContentEncoding), tc.wantEncoding)
	assertOptionalHeader(t, response, fiber.HeaderContentRange, tc.wantContentRange)
	assertOptionalHeader(t, response, fiber.HeaderContentLength, tc.wantContentLength)
}

func assertProtocolMatrixBody(t *testing.T, response *http.Response, want string) {
	t.Helper()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != want {
		t.Fatalf("expected body %q, got %q", want, string(body))
	}
}

func assertOptionalHeader(t *testing.T, response *http.Response, key, want string) {
	t.Helper()
	if want == "" {
		return
	}
	assertHeaderEquals(t, key, response.Header.Get(key), want)
}

func assertHeaderEquals(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("expected %s %q, got %q", name, want, got)
	}
}

func assertStatus(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("expected status %d, got %d", want, got)
	}
}

func newProtocolMatrixServer(t *testing.T) (string, string, string, string) {
	t.Helper()

	app, etag, variantETag, lastModified := newProtocolMatrixApp(t)
	listenerConfig := net.ListenConfig{}
	listener, err := listenerConfig.Listen(context.Background(), "tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true})
	}()

	t.Cleanup(func() {
		if err := app.Shutdown(); err != nil {
			t.Fatalf("shutdown protocol matrix app: %v", err)
		}
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, net.ErrClosed) {
				t.Fatalf("protocol matrix app listener failed: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("protocol matrix app listener did not shut down")
		}
	})

	return "http://" + listener.Addr().String(), etag, variantETag, lastModified
}

func newProtocolMatrixApp(t *testing.T) (*fiber.App, string, string, string) {
	t.Helper()

	root := t.TempDir()
	assetPath := filepath.Join(root, "app.js")
	variantPath := filepath.Join(root, "app.js.br")
	if err := os.WriteFile(assetPath, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(variantPath, []byte("br"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Root = root

	modifiedAt := time.Unix(1_720_000_200, 0).UTC()
	etag := "\"hash-app\""
	variantETag := "\"hash-app-br\""

	cat := catalog.NewInMemoryCatalog()
	upsertAssetForTest(t, cat, &catalog.Asset{
		Path:       "app.js",
		FullPath:   assetPath,
		Size:       10,
		MediaType:  "application/javascript",
		SourceHash: "hash-app",
		ETag:       etag,
		Metadata: cxmapping.NewMapFrom(map[string]string{
			"mtime_unix": "1720000200",
		}),
	})
	upsertVariantForTest(t, cat, &catalog.Variant{
		ID:           "app.js|encoding=br",
		AssetPath:    "app.js",
		ArtifactPath: variantPath,
		Size:         2,
		MediaType:    "application/javascript",
		SourceHash:   "hash-app",
		ETag:         variantETag,
		Encoding:     "br",
		Metadata: cxmapping.NewMapFrom(map[string]string{
			"stage":      "compression",
			"mtime_unix": "1720000200",
		}),
	})

	app := newHTTPTestApp(
		t,
		&cfg,
		slog.New(slog.DiscardHandler),
		cat,
		assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, slog.New(slog.DiscardHandler)),
		resolver.NewResolverForTest(&cfg.Assets, cat, slog.New(slog.DiscardHandler)),
	)
	return app, etag, variantETag, modifiedAt.Format(http.TimeFormat)
}
