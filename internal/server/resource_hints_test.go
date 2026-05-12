package server_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
)

func TestHTMLRouteEmitsResourceHintLinks(t *testing.T) {
	root := t.TempDir()
	htmlPath := filepath.Join(root, "index.html")
	body := `<!doctype html>
<html>
<head>
  <link rel="stylesheet" href="/assets/app-DiwrgTda.css">
  <link rel="preload" href="/fonts/ui-a1B2c3D4.woff2" as="font">
  <link rel="prefetch" href="/routes/about-BecOYeVz.js" as="script">
</head>
<body>
  <script type="module" src="/assets/app-DiwrgTda.js"></script>
</body>
</html>`
	if err := os.WriteFile(htmlPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Root = root
	cfg.Frontend.ResourceHints.Enable = true
	cfg.Frontend.ResourceHints.EarlyHints = false
	cfg.Frontend.ResourceHints.MaxLinks = 8
	cfg.Frontend.ResourceHints.MaxHeaderBytes = 2048

	cat := catalog.NewInMemoryCatalog()
	upsertAssetForTest(t, cat, &catalog.Asset{
		Path:       "index.html",
		FullPath:   htmlPath,
		Size:       int64(len(body)),
		MediaType:  "text/html; charset=utf-8",
		SourceHash: "hash-index",
		ETag:       "\"hash-index\"",
	})

	app := newHTTPTestApp(
		t,
		&cfg,
		slog.New(slog.DiscardHandler),
		cat,
		assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, slog.New(slog.DiscardHandler)),
		resolver.NewResolverForTest(&cfg.Assets, cat, slog.New(slog.DiscardHandler)),
	)
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closeHTTPBody(t, response)

	link := response.Header.Get("Link")
	assertLinkContains(t, link, "</assets/app-DiwrgTda.css>; rel=preload; as=style")
	assertLinkContains(t, link, "</fonts/ui-a1B2c3D4.woff2>; rel=preload; as=font; crossorigin")
	assertLinkContains(t, link, "</routes/about-BecOYeVz.js>; rel=prefetch; as=script")
	assertLinkContains(t, link, "</assets/app-DiwrgTda.js>; rel=modulepreload")
}

func assertLinkContains(t *testing.T, link, want string) {
	t.Helper()
	if !strings.Contains(link, want) {
		t.Fatalf("expected Link header to contain %q, got %q", want, link)
	}
}
