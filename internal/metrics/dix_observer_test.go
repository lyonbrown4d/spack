package metrics_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/metrics"
)

func TestObserverCachesInstruments(t *testing.T) {
	obs := &countingObservability{}
	observer := metrics.NewObserver(obs)

	buildEvent := dix.BuildEvent{
		Meta:     dix.AppMeta{Name: "spack", Version: "test"},
		Profile:  dix.ProfileProd,
		Duration: 10 * time.Millisecond,
	}

	observer.OnBuild(t.Context(), buildEvent)
	observer.OnBuild(t.Context(), buildEvent)

	if got := obs.counterCallsFor("dix_build_total"); got != 1 {
		t.Fatalf("expected dix_build_total counter to be created once, got %d", got)
	}
	if got := obs.histogramCallsFor("dix_build_duration_ms"); got != 1 {
		t.Fatalf("expected dix_build_duration_ms histogram to be created once, got %d", got)
	}
	if got := obs.histogramCallsFor("dix_build_modules"); got != 1 {
		t.Fatalf("expected dix_build_modules histogram to be created once, got %d", got)
	}
}

type countingObservability struct {
	mu             sync.Mutex
	counterCalls   map[string]int
	histogramCalls map[string]int
}

func (c *countingObservability) Logger() *slog.Logger {
	return slog.Default()
}

func (c *countingObservability) StartSpan(
	ctx context.Context,
	_ string,
	_ ...observabilityx.Attribute,
) (context.Context, observabilityx.Span) {
	return ctx, countingSpan{}
}

func (c *countingObservability) Counter(spec observabilityx.CounterSpec) observabilityx.Counter {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counterCalls == nil {
		c.counterCalls = make(map[string]int)
	}
	c.counterCalls[spec.Name]++
	return countingCounter{}
}

func (c *countingObservability) UpDownCounter(observabilityx.UpDownCounterSpec) observabilityx.UpDownCounter {
	return countingUpDownCounter{}
}

func (c *countingObservability) Histogram(spec observabilityx.HistogramSpec) observabilityx.Histogram {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.histogramCalls == nil {
		c.histogramCalls = make(map[string]int)
	}
	c.histogramCalls[spec.Name]++
	return countingHistogram{}
}

func (c *countingObservability) Gauge(observabilityx.GaugeSpec) observabilityx.Gauge {
	return countingGauge{}
}

func (c *countingObservability) counterCallsFor(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counterCalls[name]
}

func (c *countingObservability) histogramCallsFor(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.histogramCalls[name]
}

type countingCounter struct{}

func (countingCounter) Add(context.Context, int64, ...observabilityx.Attribute) {}

type countingUpDownCounter struct{}

func (countingUpDownCounter) Add(context.Context, int64, ...observabilityx.Attribute) {}

type countingHistogram struct{}

func (countingHistogram) Record(context.Context, float64, ...observabilityx.Attribute) {}

type countingGauge struct{}

func (countingGauge) Set(context.Context, float64, ...observabilityx.Attribute) {}

type countingSpan struct{}

func (countingSpan) End() {}

func (countingSpan) RecordError(error) {}

func (countingSpan) SetAttributes(...observabilityx.Attribute) {}
