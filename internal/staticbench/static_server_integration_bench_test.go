//go:build integration

package staticbench

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/server"
)

const (
	benchRequestTimeout = 10 * time.Second
	benchReadyTimeout   = 15 * time.Second
	benchWarmupRequests = 64
	nginxImage          = "nginx@sha256:d565d19ef132a5834f5897f602831ad2e40a36c26c625f2f94f9b3fdf0ed292d"
)

type staticBenchCase struct {
	name      string
	path      string
	mediaType string
	size      int64
}

type staticBenchServer struct {
	name string
	url  string
}

func BenchmarkStaticFileServers(b *testing.B) {
	root, cases := prepareStaticBenchAssets(b)
	starters := []struct {
		name  string
		start func(*testing.B, string, []staticBenchCase) staticBenchServer
	}{
		{name: "spack", start: startSpackServer},
		{name: "nginx", start: startNginxServer},
	}

	for _, starter := range starters {
		for _, benchCase := range cases {
			b.Run(starter.name+"/"+benchCase.name, func(b *testing.B) {
				benchServer := starter.start(b, root, cases)
				runStaticHTTPBenchmark(b, benchServer, benchCase)
			})
		}
	}
}

func prepareStaticBenchAssets(b *testing.B) (string, []staticBenchCase) {
	b.Helper()

	root := b.TempDir()
	cases := []staticBenchCase{
		{name: "4KiB_text", path: "small.txt", mediaType: "text/plain; charset=utf-8", size: 4 * 1024},
		{name: "128KiB_js", path: "app.js", mediaType: "application/javascript", size: 128 * 1024},
		{name: "1MiB_binary", path: "large.bin", mediaType: "application/octet-stream", size: 1024 * 1024},
	}
	for _, benchCase := range cases {
		payload := repeatedPayload(benchCase.size)
		if err := os.WriteFile(filepath.Join(root, benchCase.path), payload, 0o644); err != nil {
			b.Fatal(err)
		}
	}
	return root, cases
}

func repeatedPayload(size int64) []byte {
	block := []byte("spack-static-benchmark-payload-0123456789\n")
	repeated := bytes.Repeat(block, int(size)/len(block)+1)
	return repeated[:size]
}

func startSpackServer(b *testing.B, root string, cases []staticBenchCase) staticBenchServer {
	b.Helper()

	port := freeTCPPort(b)
	cfg := config.DefaultConfigForTest()
	cfg.Debug.Enable = false
	cfg.Assets.Root = root
	cfg.HTTP.Port = port
	cfg.HTTP.MemoryCache.Enable = false
	cfg.HTTP.MemoryCache.Warmup = false
	cfg.Compression.Enable = false
	cfg.Compression.Mode = config.CompressionModeOff
	cfg.Image.Enable = false
	cfg.Frontend.ResourceHints.Enable = false

	cat := catalog.NewInMemoryCatalog()
	for _, benchCase := range cases {
		if err := cat.UpsertAsset(&catalog.Asset{
			Path:       benchCase.path,
			FullPath:   filepath.Join(root, benchCase.path),
			Size:       benchCase.size,
			MediaType:  benchCase.mediaType,
			SourceHash: "static-bench-" + benchCase.path,
			ETag:       strconv.Quote("static-bench-" + benchCase.path),
		}); err != nil {
			b.Fatal(err)
		}
	}

	logger := slog.New(slog.DiscardHandler)
	bodyCache := assetcache.NewCacheForTest(cfg.HTTP.MemoryCache, logger)
	assetResolver := resolver.NewResolverForTest(&cfg.Assets, cat, logger)
	app, err := server.NewPreparedAppForTest(&cfg, logger, cat, bodyCache, assetResolver, nil)
	if err != nil {
		b.Fatal(err)
	}
	listenErr := make(chan error, 1)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	go func() {
		listenErr <- app.Listen(addr, fiber.ListenConfig{DisableStartupMessage: true})
	}()

	benchServer := staticBenchServer{name: "spack", url: "http://" + addr}
	waitStaticHTTPReady(b, benchServer.url+"/"+cases[0].path, listenErr)
	b.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := app.ShutdownWithContext(ctx); err != nil {
			b.Logf("shutdown spack benchmark server: %v", err)
		}
	})
	return benchServer
}

func startNginxServer(b *testing.B, root string, cases []staticBenchCase) staticBenchServer {
	b.Helper()

	if err := exec.Command("docker", "version", "--format", "{{.Server.Version}}").Run(); err != nil {
		b.Skipf("docker is required for nginx benchmark: %v", err)
	}

	port := freeTCPPort(b)
	nginxConf := filepath.Join(b.TempDir(), "nginx.conf")
	if err := os.WriteFile(nginxConf, []byte(nginxBenchmarkConfig()), 0o644); err != nil {
		b.Fatal(err)
	}

	containerName := fmt.Sprintf("spack-staticbench-%d", time.Now().UnixNano())
	args := []string{
		"run", "--rm", "-d",
		"--name", containerName,
		"-p", fmt.Sprintf("127.0.0.1:%d:8080", port),
		"-v", root + ":/usr/share/nginx/html:ro",
		"-v", nginxConf + ":/etc/nginx/nginx.conf:ro",
		nginxImage,
	}
	output, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		b.Skipf("start nginx benchmark container: %v\n%s", err, strings.TrimSpace(string(output)))
	}
	containerID := strings.TrimSpace(string(output))
	b.Cleanup(func() {
		if err := exec.Command("docker", "rm", "-f", containerID).Run(); err != nil {
			b.Logf("remove nginx benchmark container: %v", err)
		}
	})

	benchServer := staticBenchServer{name: "nginx", url: fmt.Sprintf("http://127.0.0.1:%d", port)}
	waitStaticHTTPReady(b, benchServer.url+"/"+cases[0].path, nil)
	return benchServer
}

func nginxBenchmarkConfig() string {
	return `events {}

http {
    access_log off;
    sendfile on;
    tcp_nopush on;

    server {
        listen 8080;
        root /usr/share/nginx/html;

        location / {
            try_files $uri =404;
        }
    }
}
`
}

func runStaticHTTPBenchmark(b *testing.B, benchServer staticBenchServer, benchCase staticBenchCase) {
	b.Helper()

	transport := &http.Transport{
		MaxIdleConns:        256,
		MaxIdleConnsPerHost: 256,
		DisableCompression:  true,
	}
	client := &http.Client{
		Timeout:   benchRequestTimeout,
		Transport: transport,
	}
	b.Cleanup(transport.CloseIdleConnections)

	url := benchServer.url + "/" + benchCase.path
	for range benchWarmupRequests {
		doStaticBenchmarkRequest(b, client, url, benchCase.size)
	}

	b.ReportAllocs()
	b.SetBytes(benchCase.size)
	b.ResetTimer()
	for b.Loop() {
		doStaticBenchmarkRequest(b, client, url, benchCase.size)
	}
}

func doStaticBenchmarkRequest(b *testing.B, client *http.Client, url string, expectedSize int64) {
	b.Helper()

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	if err != nil {
		b.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		b.Fatal(err)
	}
	defer closeResponseBody(b, response.Body)

	if response.StatusCode != http.StatusOK {
		b.Fatalf("expected 200 from %s, got %d", url, response.StatusCode)
	}
	written, err := io.Copy(io.Discard, response.Body)
	if err != nil {
		b.Fatal(err)
	}
	if written != expectedSize {
		b.Fatalf("expected %d response bytes from %s, got %d", expectedSize, url, written)
	}
}

func waitStaticHTTPReady(b *testing.B, url string, listenErr <-chan error) {
	b.Helper()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(benchReadyTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if listenErr != nil {
			select {
			case err := <-listenErr:
				b.Fatalf("benchmark server stopped before becoming ready: %v", err)
			default:
			}
		}

		response, err := client.Get(url)
		if err == nil {
			_, copyErr := io.Copy(io.Discard, response.Body)
			closeErr := response.Body.Close()
			if response.StatusCode == http.StatusOK && copyErr == nil && closeErr == nil {
				return
			}
			lastErr = fmt.Errorf("status=%d copy=%v close=%v", response.StatusCode, copyErr, closeErr)
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	b.Fatalf("benchmark server was not ready at %s after %s: %v", url, benchReadyTimeout, lastErr)
}

func freeTCPPort(b *testing.B) int {
	b.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := listener.Close(); err != nil {
			b.Fatal(err)
		}
	}()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		b.Fatalf("expected TCP addr, got %T", listener.Addr())
	}
	return addr.Port
}

func closeResponseBody(b *testing.B, body io.Closer) {
	b.Helper()
	if err := body.Close(); err != nil {
		b.Fatal(err)
	}
}
