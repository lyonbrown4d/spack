package main

import (
	"bufio"
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/samber/lo"
)

func optionalBenchResult(path string) (benchResult, bool) {
	if strings.TrimSpace(path) == "" || !repoFileExists(path) {
		return nil, false
	}
	return readBenchResult(path), true
}

func readBenchResult(path string) benchResult {
	samples := scanBenchSamples(readRepoFile(path))
	out := benchResult{}
	for name, sample := range samples {
		out[name] = averageSample(sample)
	}
	return out
}

func scanBenchSamples(body []byte) map[string]*benchSample {
	samples := map[string]*benchSample{}
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		name, metrics, ok := parseBenchmarkLine(scanner.Text())
		if ok {
			addBenchSample(samples, name, metrics)
		}
	}
	if err := scanner.Err(); err != nil {
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

func parseBenchmarkLine(line string) (string, map[string]float64, bool) {
	fields := strings.Fields(line)
	if len(fields) < 4 || !strings.HasPrefix(fields[0], "Benchmark") {
		return "", nil, false
	}
	metrics := parseBenchmarkMetrics(fields[2:])
	return stripBenchmarkSuffix(fields[0]), metrics, len(metrics) > 0
}

func parseBenchmarkMetrics(fields []string) map[string]float64 {
	metrics := map[string]float64{}
	for index := 0; index+1 < len(fields); index += 2 {
		value, err := strconv.ParseFloat(fields[index], 64)
		if err != nil {
			continue
		}
		if metric, ok := metricName(fields[index+1]); ok {
			metrics[metric] = value
		}
	}
	return metrics
}

func stripBenchmarkSuffix(name string) string {
	index := strings.LastIndexByte(name, '-')
	if index <= 0 {
		return name
	}
	if _, err := strconv.Atoi(name[index+1:]); err != nil {
		return name
	}
	return name[:index]
}

func metricName(unit string) (string, bool) {
	switch unit {
	case "ns/op":
		return "ns_per_op", true
	case "B/op":
		return "b_per_op", true
	case "allocs/op":
		return "allocs_per_op", true
	default:
		return "", false
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
