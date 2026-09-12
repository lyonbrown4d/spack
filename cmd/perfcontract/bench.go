package main

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/samber/lo"
	"golang.org/x/perf/benchfmt"
)

func optionalBenchResult(path string) (benchResult, bool) {
	if strings.TrimSpace(path) == "" || !repoFileExists(path) {
		return nil, false
	}
	return readBenchResult(path), true
}

func readBenchResult(path string) benchResult {
	return parseBenchResult(readRepoFile(path))
}

func parseBenchResult(body []byte) benchResult {
	samples := scanBenchSamples(body)
	out := benchResult{}
	for name, sample := range samples {
		out[name] = averageSample(sample)
	}
	return out
}

func scanBenchSamples(body []byte) map[string]*benchSample {
	samples := map[string]*benchSample{}
	reader := benchfmt.NewReader(bytes.NewReader(body), "<benchmark>")
	for reader.Scan() {
		result, ok := reader.Result().(*benchfmt.Result)
		if !ok {
			continue
		}
		metrics := benchmarkMetrics(result.Values)
		if len(metrics) > 0 {
			addBenchSample(samples, benchmarkName(result.Name), metrics)
		}
	}
	if err := reader.Err(); err != nil {
		fatalf("scan benchmark output: %v", err)
	}
	return samples
}

func addBenchSample(samples map[string]*benchSample, name string, metrics map[string]float64) {
	sample := samples[name]
	if sample == nil {
		sample = &benchSample{sum: map[string]float64{}}
		samples[name] = sample
	}
	sample.count++
	for metric, value := range metrics {
		sample.sum[metric] += value
	}
}

func averageSample(sample *benchSample) map[string]float64 {
	out := map[string]float64{}
	for metric, value := range sample.sum {
		out[metric] = value / float64(sample.count)
	}
	return out
}

func benchmarkName(name benchfmt.Name) string {
	base, parts := name.Parts()
	out := make([]byte, 0, len("Benchmark")+len(name))
	out = append(out, "Benchmark"...)
	out = append(out, base...)
	for _, part := range parts {
		if part[0] != '-' {
			out = append(out, part...)
		}
	}
	return string(out)
}

func benchmarkMetrics(values []benchfmt.Value) map[string]float64 {
	metrics := map[string]float64{}
	for _, value := range values {
		metric, metricValue, ok := benchmarkMetric(value)
		if ok {
			metrics[metric] = metricValue
		}
	}
	return metrics
}

func benchmarkMetric(value benchfmt.Value) (string, float64, bool) {
	switch value.OrigUnit {
	case "ns/op":
		return "ns_per_op", value.OrigValue, true
	case "B/op":
		return "b_per_op", value.OrigValue, true
	case "allocs/op":
		return "allocs_per_op", value.OrigValue, true
	}

	switch value.Unit {
	case "sec/op":
		return "ns_per_op", value.Value * 1e9, true
	case "B/op":
		return "b_per_op", value.Value, true
	case "allocs/op":
		return "allocs_per_op", value.Value, true
	default:
		return "", 0, false
	}
}

func checkGoBenchmarks(
	budgets []goBenchBudget,
	baseline benchResult,
	hasBaseline bool,
	candidate benchResult,
) ([]checkRow, bool) {
	failed := false
	rows := lo.FlatMap(budgets, func(budget goBenchBudget, _ int) []checkRow {
		budgetRows, budgetFailed := checkGoBenchmark(budget, baseline, hasBaseline, candidate)
		failed = failed || budgetFailed
		return budgetRows
	})
	return rows, failed
}

func checkGoBenchmark(
	budget goBenchBudget,
	baseline benchResult,
	hasBaseline bool,
	candidate benchResult,
) ([]checkRow, bool) {
	candidateMetrics, ok := candidate[budget.Benchmark]
	if !ok {
		return missingCandidateRows(budget), true
	}
	return checkGoBenchmarkMetrics(budget, baseline, hasBaseline, candidateMetrics)
}

func missingCandidateRows(budget goBenchBudget) []checkRow {
	return lo.Map(sortedMetricNames(budget.Metrics), func(metric string, _ int) checkRow {
		return newCheckRow(budget, metric, 0, 0, 0, renderBudget(budget.Metrics[metric]), "missing candidate")
	})
}

func checkGoBenchmarkMetrics(
	budget goBenchBudget,
	baseline benchResult,
	hasBaseline bool,
	candidateMetrics map[string]float64,
) ([]checkRow, bool) {
	failed := false
	rows := lo.Map(sortedMetricNames(budget.Metrics), func(metric string, _ int) checkRow {
		row, rowFailed := evaluateMetric(budget, metric, baseline, hasBaseline, candidateMetrics)
		failed = failed || rowFailed
		return row
	})
	return rows, failed
}

func evaluateMetric(
	budget goBenchBudget,
	metric string,
	baseline benchResult,
	hasBaseline bool,
	candidateMetrics map[string]float64,
) (checkRow, bool) {
	limit := budget.Metrics[metric]
	candidateValue, ok := candidateMetrics[metric]
	if !ok {
		return newCheckRow(budget, metric, 0, 0, 0, renderBudget(limit), "missing candidate"), true
	}
	baseValue := baselineMetricValue(baseline, budget.Benchmark, metric, hasBaseline)
	delta := regressionPercent(baseValue, candidateValue)
	status := metricStatus(limit, candidateValue, baseValue, delta, hasBaseline)
	return newCheckRow(budget, metric, baseValue, candidateValue, delta, renderBudget(limit), status), status == "fail"
}

func baselineMetricValue(baseline benchResult, benchmark, metric string, hasBaseline bool) float64 {
	if !hasBaseline {
		return 0
	}
	return baseline[benchmark][metric]
}

func regressionPercent(baseValue, candidateValue float64) float64 {
	if baseValue <= 0 {
		return 0
	}
	return ((candidateValue - baseValue) / baseValue) * 100
}

func metricStatus(limit metricBudget, candidateValue, baseValue, delta float64, hasBaseline bool) string {
	if limit.Max != nil && candidateValue > *limit.Max {
		return "fail"
	}
	if !hasBaseline {
		return "no baseline"
	}
	if baseValue <= 0 {
		return "missing baseline"
	}
	if limit.MaxRegressionPercent != nil && delta > *limit.MaxRegressionPercent {
		return "fail"
	}
	return "pass"
}

func newCheckRow(
	budget goBenchBudget,
	metric string,
	baseValue float64,
	candidateValue float64,
	delta float64,
	budgetText string,
	status string,
) checkRow {
	return checkRow{
		ID:        budget.ID,
		Scenario:  budget.Scenario,
		Benchmark: budget.Benchmark,
		Metric:    metric,
		Base:      baseValue,
		Candidate: candidateValue,
		Delta:     delta,
		Budget:    budgetText,
		Status:    status,
	}
}

func renderBudget(budget metricBudget) string {
	parts := lo.Compact([]string{
		lo.TernaryF(budget.MaxRegressionPercent != nil,
			func() string { return fmt.Sprintf("regression <= %.2f%%", *budget.MaxRegressionPercent) },
			func() string { return "" },
		),
		lo.TernaryF(budget.Max != nil,
			func() string { return fmt.Sprintf("max <= %.2f", *budget.Max) },
			func() string { return "" },
		),
	})
	if len(parts) == 0 {
		return "record only"
	}
	return strings.Join(parts, ", ")
}

func sortedMetricNames(metrics map[string]metricBudget) []string {
	return slices.Sorted(maps.Keys(metrics))
}
