package server_test

import (
	"testing"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/server"
)

func TestRegisterHealthCheckSetupRegistersDixReports(t *testing.T) {
	cfg := config.DefaultConfigForTest()
	cfg.Assets.Root = t.TempDir()

	cat := catalog.NewInMemoryCatalog()
	runtime, err := dix.New("server-health-test",
		dix.WithModules(
			server.NewHealthModuleForTest(&cfg, cat),
		),
	).Build()
	if err != nil {
		t.Fatal(err)
	}

	assertHealthyDixReport(t, runtime.CheckHealth(t.Context()), dix.HealthKindGeneral, "catalog")
	assertHealthyDixReport(t, runtime.CheckLiveness(t.Context()), dix.HealthKindLiveness, "server")
	assertHealthyDixReport(t, runtime.CheckReadiness(t.Context()), dix.HealthKindReadiness, "assets_root")
}

func assertHealthyDixReport(t *testing.T, report dix.HealthReport, kind dix.HealthKind, checkName string) {
	t.Helper()

	if !report.Healthy() {
		t.Fatalf("expected %s report to be healthy, got %v", kind, report.Error())
	}
	if report.Kind != kind {
		t.Fatalf("expected health kind %q, got %q", kind, report.Kind)
	}
	if err, ok := report.Checks.Get(checkName); !ok || err != nil {
		t.Fatalf("expected %s check %q to pass, got ok=%v err=%v", kind, checkName, ok, err)
	}
}
