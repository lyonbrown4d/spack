package cmdkit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/arcgolabs/dix"
	obsprom "github.com/arcgolabs/observabilityx/prometheus"
	"github.com/lyonbrown4d/spack/internal/artifact"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/cmdkit"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/event"
	"github.com/lyonbrown4d/spack/internal/pipeline"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/lyonbrown4d/spack/internal/server"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
)

func mustCreatePrometheusTestContainer(t *testing.T) *dix.App {
	t.Helper()

	app, err := cmdkit.CreateContainer(
		config.LoadOptions{},
		asyncx.Module,
		event.Module,
		source.Module,
		sourcecatalog.Module,
		artifact.Module,
		contentcoding.Module,
		assetcache.Module,
		pipeline.Module,
		resolver.Module,
		server.Module,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got := app.RunStopTimeout(); got != dix.DefaultRunStopTimeout {
		t.Fatalf("expected run stop timeout %s, got %s", dix.DefaultRunStopTimeout, got)
	}

	return app
}

func mustBuildPrometheusRuntime(t *testing.T, app *dix.App) *dix.Runtime {
	t.Helper()

	rt, err := app.Build()
	if err != nil {
		t.Fatal(err)
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		t.Fatal("expected build info to be available")
	}
	if got := rt.Meta().Version; got != info.Main.Version {
		t.Fatalf("expected runtime version %q, got %q", info.Main.Version, got)
	}
	if recorder := rt.EventRecorder(); recorder == nil || recorder.Capacity() != 128 {
		t.Fatalf("expected runtime recent event recorder with capacity 128, got %#v", recorder)
	}

	return rt
}

func prometheusBodyUntilDixMetrics(t *testing.T, adapter *obsprom.Adapter) string {
	t.Helper()

	body := ""
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/prometheus", http.NoBody)
		response := httptest.NewRecorder()
		adapter.Handler().ServeHTTP(response, request)

		body = response.Body.String()
		if strings.Contains(body, "spack_dix_build_total") && strings.Contains(body, `app="spack"`) {
			return body
		}
		time.Sleep(10 * time.Millisecond)
	}

	return body
}

func TestCreateContainerBuildPublishesDixMetrics(t *testing.T) {
	t.Setenv("SPACK_ASSETS_ROOT", t.TempDir())
	t.Setenv("SPACK_LOGGER_CONSOLE_ENABLED", "false")

	app := mustCreatePrometheusTestContainer(t)
	rt := mustBuildPrometheusRuntime(t, app)

	adapter, err := dix.ResolveAs[*obsprom.Adapter](rt.Container())
	if err != nil {
		t.Fatal(err)
	}

	body := prometheusBodyUntilDixMetrics(t, adapter)
	if !strings.Contains(body, "spack_dix_build_total") {
		t.Fatalf("expected dix build metric to be exported, got body:\n%s", body)
	}
	if !strings.Contains(body, `app="spack"`) {
		t.Fatalf("expected dix metrics to include app label, got body:\n%s", body)
	}
}
