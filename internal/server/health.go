package server

import (
	"context"
	"errors"
	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/observabilityx"
	"github.com/gofiber/fiber/v3"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/oops"
	"os"
	"strings"
	"time"
)

var (
	healthCheckRunsTotalSpec = observabilityx.NewCounterSpec(
		"health_check_runs_total",
		observabilityx.WithDescription("Total number of health check executions."),
		observabilityx.WithLabelKeys("kind", "check", "result"),
	)
	healthCheckDurationSpec = observabilityx.NewHistogramSpec(
		"health_check_duration_seconds",
		observabilityx.WithDescription("Health check execution duration in seconds."),
		observabilityx.WithUnit("s"),
		observabilityx.WithLabelKeys("kind", "check", "result"),
	)
	healthReportsTotalSpec = observabilityx.NewCounterSpec(
		"health_reports_total",
		observabilityx.WithDescription("Total number of aggregated health report generations."),
		observabilityx.WithLabelKeys("kind", "result"),
	)
	healthReportDurationSpec = observabilityx.NewHistogramSpec(
		"health_report_duration_seconds",
		observabilityx.WithDescription("Aggregated health report generation duration in seconds."),
		observabilityx.WithUnit("s"),
		observabilityx.WithLabelKeys("kind", "result"),
	)
)

const (
	healthEndpoint    = "/healthz"
	livenessEndpoint  = "/livez"
	readinessEndpoint = "/readyz"
)

type healthCheckDefinition struct {
	Name  string
	Kind  dix.HealthKind
	Check dix.HealthCheckFunc
}

func newHealthCheckDefinitions(cfg *config.Config, cat catalog.Catalog) *cxlist.List[healthCheckDefinition] {
	return cxlist.NewList[healthCheckDefinition](
		newHealthCheckDefinition(dix.HealthKindGeneral, "catalog", func(context.Context) error {
			return checkCatalog(cat)
		}),
		newHealthCheckDefinition(dix.HealthKindLiveness, "server", func(context.Context) error {
			return nil
		}),
		newHealthCheckDefinition(dix.HealthKindReadiness, "assets_root", func(context.Context) error {
			return checkAssetsRoot(cfg.Assets.Root)
		}),
	)
}

func newHealthCheckDefinition(kind dix.HealthKind, name string, check dix.HealthCheckFunc) healthCheckDefinition {
	return healthCheckDefinition{
		Name:  strings.TrimSpace(name),
		Kind:  kind,
		Check: check,
	}
}

func registerHealthCheckSetup(container *dix.Container, _ dix.Lifecycle) error {
	checks, err := dix.ResolveAs[*cxlist.List[healthCheckDefinition]](container)
	if err != nil {
		return err
	}

	checks.Range(func(_ int, check healthCheckDefinition) bool {
		if check.Name == "" || check.Check == nil {
			return true
		}

		switch check.Kind {
		case dix.HealthKindGeneral:
			container.RegisterHealthCheck(check.Name, check.Check)
		case dix.HealthKindLiveness:
			container.RegisterLivenessCheck(check.Name, check.Check)
		case dix.HealthKindReadiness:
			container.RegisterReadinessCheck(check.Name, check.Check)
		default:
			container.RegisterHealthCheck(check.Name, check.Check)
		}
		return true
	})
	return nil
}

func registerHealthRoutes(
	app *fiber.App,
	checks *cxlist.List[healthCheckDefinition],
	obs observabilityx.Observability,
) {
	app.Get(healthEndpoint, healthHandler(dix.HealthKindGeneral, checks, obs))
	app.Get(livenessEndpoint, healthHandler(dix.HealthKindLiveness, checks, obs))
	app.Get(readinessEndpoint, healthHandler(dix.HealthKindReadiness, checks, obs))
}

func healthHandler(kind dix.HealthKind, checks *cxlist.List[healthCheckDefinition], obs observabilityx.Observability) fiber.Handler {
	obs = observabilityx.Normalize(obs, nil)

	return func(c fiber.Ctx) error {
		startedAt := time.Now()
		report := runHealthChecks(c.Context(), kind, checks, obs)
		status := fiber.StatusOK
		if !report.Healthy() {
			status = fiber.StatusServiceUnavailable
		}
		recordHealthReportMetrics(c.Context(), obs, kind, report.Healthy(), startedAt)
		c.Status(status)
		return c.JSON(report)
	}
}

func runHealthChecks(
	ctx context.Context,
	kind dix.HealthKind,
	checks *cxlist.List[healthCheckDefinition],
	obs observabilityx.Observability,
) dix.HealthReport {
	matched := cxlist.FilterList(checks, func(_ int, check healthCheckDefinition) bool {
		return check.Kind == kind && check.Name != "" && check.Check != nil
	})
	report := dix.HealthReport{
		Kind:   kind,
		Checks: cxmapping.NewMapWithCapacity[string, error](matched.Len()),
	}
	matched.Range(func(_ int, check healthCheckDefinition) bool {
		startedAt := time.Now()
		err := check.Check(ctx)
		recordHealthCheckMetrics(ctx, obs, kind, check.Name, err == nil, startedAt)
		report.Checks.Set(check.Name, err)
		return true
	})
	return report
}

func recordHealthCheckMetrics(
	ctx context.Context,
	obs observabilityx.Observability,
	kind dix.HealthKind,
	checkName string,
	healthy bool,
	startedAt time.Time,
) {
	obs = observabilityx.Normalize(obs, nil)
	attrs := cxlist.NewList(
		observabilityx.String("kind", string(kind)),
		observabilityx.String("check", strings.TrimSpace(checkName)),
		observabilityx.String("result", healthMetricResult(healthy)))
	attrs.ViewValues(func(values []observabilityx.Attribute) {
		obs.Counter(healthCheckRunsTotalSpec).Add(ctx, 1, values...)
		obs.Histogram(healthCheckDurationSpec).Record(ctx, time.Since(startedAt).Seconds(), values...)
	})
}

func recordHealthReportMetrics(
	ctx context.Context,
	obs observabilityx.Observability,
	kind dix.HealthKind,
	healthy bool,
	startedAt time.Time,
) {
	obs = observabilityx.Normalize(obs, nil)
	attrs := cxlist.NewList(
		observabilityx.String("kind", string(kind)),
		observabilityx.String("result", healthMetricResult(healthy)),
	)
	attrs.ViewValues(func(values []observabilityx.Attribute) {
		obs.Counter(healthReportsTotalSpec).Add(ctx, 1, values...)
		obs.Histogram(healthReportDurationSpec).Record(ctx, time.Since(startedAt).Seconds(), values...)
	})
}

func healthMetricResult(healthy bool) string {
	if healthy {
		return "ok"
	}
	return "error"
}

func checkCatalog(cat catalog.Catalog) error {
	if cat == nil {
		return oops.In("server").Owner("health").Wrap(errors.New("catalog is not configured"))
	}
	if cat.Snapshot() == nil {
		return oops.In("server").Owner("health").Wrap(errors.New("catalog snapshot is unavailable"))
	}
	return nil
}

func checkAssetsRoot(root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return oops.In("server").Owner("health").Wrap(errors.New("assets root is not configured"))
	}

	info, err := os.Stat(root)
	if err != nil {
		return oops.Wrapf(err, "stat assets root %q", root)
	}
	if info.IsDir() {
		return nil
	}
	if info.Mode().IsRegular() && spackbundle.IsBundlePath(root) {
		return nil
	}
	return oops.Errorf("assets root %q is not a directory or .spack bundle", root)
}
