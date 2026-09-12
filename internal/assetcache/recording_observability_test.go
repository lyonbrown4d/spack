package assetcache_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/config"
)

type recordingObservability struct {
	mu                   sync.Mutex
	counters             map[string]int64
	counterInstruments   map[string]int
	histogramInstruments map[string]int
}

func (r *recordingObservability) Logger() *slog.Logger {
	return slog.Default()
}

func (r *recordingObservability) StartSpan(
	ctx context.Context,
	_ string,
	_ ...observabilityx.Attribute,
) (context.Context, observabilityx.Span) {
	return ctx, recordingSpan{}
}

func (r *recordingObservability) Counter(spec observabilityx.CounterSpec) observabilityx.Counter {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.counterInstruments == nil {
		r.counterInstruments = map[string]int{}
	}
	r.counterInstruments[spec.Name]++
	return recordingCounter{name: spec.Name, obs: r}
}

func (r *recordingObservability) UpDownCounter(observabilityx.UpDownCounterSpec) observabilityx.UpDownCounter {
	return noopUpDownCounter{}
}

func (r *recordingObservability) Histogram(spec observabilityx.HistogramSpec) observabilityx.Histogram {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.histogramInstruments == nil {
		r.histogramInstruments = map[string]int{}
	}
	r.histogramInstruments[spec.Name]++
	return noopHistogram{}
}

func (r *recordingObservability) Gauge(observabilityx.GaugeSpec) observabilityx.Gauge {
	return noopGauge{}
}

type recordingCounter struct {
	name string
	obs  *recordingObservability
}

func (r recordingCounter) Add(_ context.Context, value int64, _ ...observabilityx.Attribute) {
	r.obs.mu.Lock()
	defer r.obs.mu.Unlock()

	if r.obs.counters == nil {
		r.obs.counters = map[string]int64{}
	}
	r.obs.counters[r.name] += value
}

type noopUpDownCounter struct{}

func (noopUpDownCounter) Add(context.Context, int64, ...observabilityx.Attribute) {}

type noopHistogram struct{}

func (noopHistogram) Record(context.Context, float64, ...observabilityx.Attribute) {}

type noopGauge struct{}

func (noopGauge) Set(context.Context, float64, ...observabilityx.Attribute) {}

type recordingSpan struct{}

func (recordingSpan) End() {}

func (recordingSpan) RecordError(error) {}

func (recordingSpan) SetAttributes(...observabilityx.Attribute) {}

func TestCacheMetricsReuseInstruments(t *testing.T) {
	const payload = "cached"

	root := t.TempDir()
	path := filepath.Join(root, "asset.js")
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}

	obs := &recordingObservability{}
	cache := assetcache.NewCacheWithObservabilityForTest(config.MemoryCache{
		Enable:      true,
		MaxEntries:  16,
		MaxFileSize: 1024,
		TTL:         "5m",
	}, slog.New(slog.DiscardHandler), obs)

	for range 2 {
		body, _, err := cache.GetOrLoad(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != payload {
			t.Fatalf("expected payload %q, got %q", payload, body)
		}
	}

	assertInstrumentCount(t, obs, "asset_cache_hits_total", 1)
	assertInstrumentCount(t, obs, "asset_cache_misses_total", 1)
	assertInstrumentCount(t, obs, "asset_cache_fills_total", 1)
	assertHistogramInstrumentCount(t, obs, "asset_cache_warmup_duration_seconds", 1)
}

func assertCounterValue(t *testing.T, obs *recordingObservability, name string, want int64) {
	t.Helper()

	obs.mu.Lock()
	defer obs.mu.Unlock()

	got := obs.counters[name]
	if got != want {
		t.Fatalf("expected counter %s=%d, got %d", name, want, got)
	}
}

func assertInstrumentCount(t *testing.T, obs *recordingObservability, name string, want int) {
	t.Helper()

	obs.mu.Lock()
	defer obs.mu.Unlock()

	if got := obs.counterInstruments[name]; got != want {
		t.Fatalf("expected counter instrument %s to be requested %d time(s), got %d", name, want, got)
	}
}

func assertHistogramInstrumentCount(t *testing.T, obs *recordingObservability, name string, want int) {
	t.Helper()

	obs.mu.Lock()
	defer obs.mu.Unlock()

	if got := obs.histogramInstruments[name]; got != want {
		t.Fatalf("expected histogram instrument %s to be requested %d time(s), got %d", name, want, got)
	}
}
