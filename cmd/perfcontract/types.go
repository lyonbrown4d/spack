package main

type budgetFile struct {
	Schema       int             `json:"schema"`
	Updated      string          `json:"updated"`
	GoBenchmarks []goBenchBudget `json:"go_benchmarks"`
}

type goBenchBudget struct {
	ID        string                  `json:"id"`
	Scenario  string                  `json:"scenario"`
	Benchmark string                  `json:"benchmark"`
	Metrics   map[string]metricBudget `json:"metrics"`
}

type metricBudget struct {
	MaxRegressionPercent *float64 `json:"max_regression_percent,omitempty"`
	Max                  *float64 `json:"max,omitempty"`
}

type benchSample struct {
	count int
	sum   map[string]float64
}

type benchResult map[string]map[string]float64

type checkRow struct {
	ID        string
	Scenario  string
	Benchmark string
	Metric    string
	Base      float64
	Candidate float64
	Delta     float64
	Budget    string
	Status    string
}

type metadata struct {
	GeneratedAt string            `json:"generated_at"`
	Commit      string            `json:"commit,omitempty"`
	GoVersion   string            `json:"go_version"`
	GOOS        string            `json:"goos"`
	GOARCH      string            `json:"goarch"`
	NumCPU      int               `json:"num_cpu"`
	Hostname    string            `json:"hostname,omitempty"`
	Environment map[string]string `json:"environment"`
}

type k6Summary struct {
	File      string
	Rate      float64
	MedianMS  float64
	P95MS     float64
	P99MS     float64
	FailedPct float64
}
