package metrics

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/observabilityx"
	"github.com/samber/lo"
)

type Option func(*config)

type config struct {
	metricPrefix            string
	includeVersionAttribute bool
	includeHealthCheckName  bool
}

func NewObserver(obs observabilityx.Observability, opts ...Option) dix.Observer {
	cfg := config{
		metricPrefix:            "dix",
		includeVersionAttribute: true,
		includeHealthCheckName:  true,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	return &observer{
		obs: observabilityx.Normalize(obs, nil),
		cfg: cfg,
	}
}

func WithObservability(obs observabilityx.Observability, opts ...Option) dix.AppOption {
	return dix.WithObserver(NewObserver(obs, opts...))
}

type observer struct {
	obs observabilityx.Observability
	cfg config

	counters   sync.Map
	histograms sync.Map
}

var metricDescriptions = map[string]string{
	"build_total":              "Total number of dix application build attempts.",
	"build_duration_ms":        "dix application build duration in milliseconds.",
	"build_modules":            "Number of modules included in a dix application build.",
	"build_providers":          "Number of providers included in a dix application build.",
	"build_hooks":              "Number of hooks included in a dix application build.",
	"build_setups":             "Number of setups included in a dix application build.",
	"build_invokes":            "Number of invokes included in a dix application build.",
	"start_total":              "Total number of dix runtime start attempts.",
	"start_duration_ms":        "dix runtime start duration in milliseconds.",
	"start_registered_hooks":   "Number of registered start hooks in a dix runtime.",
	"start_completed_hooks":    "Number of completed start hooks in a dix runtime.",
	"start_rollback_total":     "Total number of dix runtime starts that triggered rollback.",
	"stop_total":               "Total number of dix runtime stop attempts.",
	"stop_duration_ms":         "dix runtime stop duration in milliseconds.",
	"stop_registered_hooks":    "Number of registered stop hooks in a dix runtime.",
	"stop_shutdown_errors":     "Number of shutdown errors observed during a dix runtime stop.",
	"stop_hook_error_total":    "Total number of dix runtime stops that encountered stop hook errors.",
	"health_check_total":       "Total number of dix health check executions.",
	"health_check_duration_ms": "dix health check execution duration in milliseconds.",
	"state_transition_total":   "Total number of dix runtime state transitions.",
}

func (o *observer) OnBuild(ctx context.Context, event dix.BuildEvent) {
	attrs := o.withResultAttrs(event.Meta, event.Profile, event.Err)
	o.counter("build_total", "app", "profile", "result", "version").
		Add(contextOrBackground(ctx), 1, attrs...)
	o.histogram("build_duration_ms", "ms", "app", "profile", "result", "version").
		Record(contextOrBackground(ctx), durationMS(event.Duration), attrs...)
	o.histogram("build_modules", "", "app", "profile", "result", "version").
		Record(contextOrBackground(ctx), float64(event.ModuleCount), attrs...)
	o.histogram("build_providers", "", "app", "profile", "result", "version").
		Record(contextOrBackground(ctx), float64(event.ProviderCount), attrs...)
	o.histogram("build_hooks", "", "app", "profile", "result", "version").
		Record(contextOrBackground(ctx), float64(event.HookCount), attrs...)
	o.histogram("build_setups", "", "app", "profile", "result", "version").
		Record(contextOrBackground(ctx), float64(event.SetupCount), attrs...)
	o.histogram("build_invokes", "", "app", "profile", "result", "version").
		Record(contextOrBackground(ctx), float64(event.InvokeCount), attrs...)
}

func (o *observer) OnStart(ctx context.Context, event dix.StartEvent) {
	attrs := o.withResultAttrs(event.Meta, event.Profile, event.Err)
	o.counter("start_total", "app", "profile", "result", "version").
		Add(contextOrBackground(ctx), 1, attrs...)
	o.histogram("start_duration_ms", "ms", "app", "profile", "result", "version").
		Record(contextOrBackground(ctx), durationMS(event.Duration), attrs...)
	o.histogram("start_registered_hooks", "", "app", "profile", "result", "version").
		Record(contextOrBackground(ctx), float64(event.StartHookCount), attrs...)
	o.histogram("start_completed_hooks", "", "app", "profile", "result", "version").
		Record(contextOrBackground(ctx), float64(event.StartedHookCount), attrs...)
	if event.RolledBack {
		o.counter("start_rollback_total", "app", "profile", "result", "version").
			Add(contextOrBackground(ctx), 1, attrs...)
	}
}

func (o *observer) OnStop(ctx context.Context, event dix.StopEvent) {
	attrs := o.withResultAttrs(event.Meta, event.Profile, event.Err)
	o.counter("stop_total", "app", "profile", "result", "version").
		Add(contextOrBackground(ctx), 1, attrs...)
	o.histogram("stop_duration_ms", "ms", "app", "profile", "result", "version").
		Record(contextOrBackground(ctx), durationMS(event.Duration), attrs...)
	o.histogram("stop_registered_hooks", "", "app", "profile", "result", "version").
		Record(contextOrBackground(ctx), float64(event.StopHookCount), attrs...)
	o.histogram("stop_shutdown_errors", "", "app", "profile", "result", "version").
		Record(contextOrBackground(ctx), float64(event.ShutdownErrorCount), attrs...)
	if event.HookError {
		o.counter("stop_hook_error_total", "app", "profile", "result", "version").
			Add(contextOrBackground(ctx), 1, attrs...)
	}
}

func (o *observer) OnHealthCheck(ctx context.Context, event dix.HealthCheckEvent) {
	attrs := lo.Concat(o.commonAttrs(event.Meta, event.Profile), []observabilityx.Attribute{
		observabilityx.String("kind", string(event.Kind)),
		observabilityx.String("result", resultOf(event.Err)),
	})
	if o.cfg.includeHealthCheckName && event.Name != "" {
		attrs = lo.Concat(attrs, []observabilityx.Attribute{observabilityx.String("check", event.Name)})
	}
	o.counter("health_check_total", "app", "check", "kind", "profile", "result", "version").
		Add(contextOrBackground(ctx), 1, attrs...)
	o.histogram("health_check_duration_ms", "ms", "app", "check", "kind", "profile", "result", "version").
		Record(contextOrBackground(ctx), durationMS(event.Duration), attrs...)
}

func (o *observer) OnStateTransition(ctx context.Context, event dix.StateTransitionEvent) {
	attrs := lo.Concat(o.commonAttrs(event.Meta, event.Profile), []observabilityx.Attribute{
		observabilityx.String("from", event.From.String()),
		observabilityx.String("to", event.To.String()),
	})
	o.counter("state_transition_total", "app", "from", "profile", "to", "version").
		Add(contextOrBackground(ctx), 1, attrs...)
}

func (o *observer) counter(suffix string, labelKeys ...string) observabilityx.Counter {
	spec := o.counterSpec(suffix, labelKeys...)
	key := fmt.Sprintf("%s|%s|%v", spec.Name, spec.Description, spec.LabelKeys)
	if cached, ok := o.counters.Load(key); ok {
		if counter, castOK := cached.(observabilityx.Counter); castOK {
			return counter
		}
	}
	instrument := o.obs.Counter(spec)
	actual, _ := o.counters.LoadOrStore(key, instrument)
	counter, castOK := actual.(observabilityx.Counter)
	if !castOK {
		return instrument
	}
	return counter
}

func (o *observer) histogram(suffix, unit string, labelKeys ...string) observabilityx.Histogram {
	spec := o.histogramSpec(suffix, unit, labelKeys...)
	key := fmt.Sprintf("%s|%s|%s|%v", spec.Name, spec.Description, spec.Unit, spec.LabelKeys)
	if cached, ok := o.histograms.Load(key); ok {
		if histogram, castOK := cached.(observabilityx.Histogram); castOK {
			return histogram
		}
	}
	instrument := o.obs.Histogram(spec)
	actual, _ := o.histograms.LoadOrStore(key, instrument)
	histogram, castOK := actual.(observabilityx.Histogram)
	if !castOK {
		return instrument
	}
	return histogram
}

func (o *observer) commonAttrs(meta dix.AppMeta, profile dix.Profile) []observabilityx.Attribute {
	attrs := []observabilityx.Attribute{
		observabilityx.String("app", meta.Name),
		observabilityx.String("profile", string(profile)),
	}
	if o.cfg.includeVersionAttribute && strings.TrimSpace(meta.Version) != "" {
		return lo.Concat(attrs, []observabilityx.Attribute{observabilityx.String("version", meta.Version)})
	}
	return attrs
}

func (o *observer) withResultAttrs(meta dix.AppMeta, profile dix.Profile, err error) []observabilityx.Attribute {
	return lo.Concat(o.commonAttrs(meta, profile), []observabilityx.Attribute{observabilityx.String("result", resultOf(err))})
}

func (o *observer) metricName(suffix string) string {
	return o.cfg.metricPrefix + "_" + suffix
}

func (o *observer) counterSpec(suffix string, labelKeys ...string) observabilityx.CounterSpec {
	return observabilityx.NewCounterSpec(
		o.metricName(suffix),
		observabilityx.WithDescription(o.metricDescription(suffix)),
		observabilityx.WithLabelKeys(labelKeys...),
	)
}

func (o *observer) histogramSpec(suffix, unit string, labelKeys ...string) observabilityx.HistogramSpec {
	spec := observabilityx.NewHistogramSpec(
		o.metricName(suffix),
		observabilityx.WithDescription(o.metricDescription(suffix)),
		observabilityx.WithLabelKeys(labelKeys...),
	)
	if strings.TrimSpace(unit) == "" {
		return spec
	}
	return observabilityx.NewHistogramSpec(
		o.metricName(suffix),
		observabilityx.WithDescription(o.metricDescription(suffix)),
		observabilityx.WithUnit(unit),
		observabilityx.WithLabelKeys(labelKeys...),
	)
}

func (o *observer) metricDescription(suffix string) string {
	return metricDescriptions[suffix]
}

func resultOf(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}

func durationMS(durationValue time.Duration) float64 {
	return float64(durationValue.Milliseconds())
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
