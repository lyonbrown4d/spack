package main

import (
	"fmt"
	"strings"
)

type reportWriter struct {
	builder strings.Builder
}

func renderReport(budgets budgetFile, rows []checkRow, k6 []k6Summary, meta metadata, hasBaseline bool) string {
	writer := reportWriter{}
	writer.metadata(budgets, meta, hasBaseline)
	writer.goBenchmarks(rows)
	writer.k6Summaries(k6)
	return writer.builder.String()
}

func (w *reportWriter) metadata(budgets budgetFile, meta metadata, hasBaseline bool) {
	w.writeString("# SPACK Performance Report\n\n")
	w.writeString("## Metadata\n\n")
	w.printf("- Generated at: `%s`\n", meta.GeneratedAt)
	w.printf("- Commit: `%s`\n", valueOrUnknown(meta.Commit))
	w.printf("- Go: `%s`\n", meta.GoVersion)
	w.printf("- Platform: `%s/%s`\n", meta.GOOS, meta.GOARCH)
	w.printf("- CPUs: `%d`\n", meta.NumCPU)
	w.printf("- Budget file schema: `%d`, updated `%s`\n", budgets.Schema, budgets.Updated)
	if !hasBaseline {
		w.writeString("- Baseline: `not provided`; relative regression checks were skipped\n")
	}
}

func (w *reportWriter) goBenchmarks(rows []checkRow) {
	w.writeString("\n## Go benchmark contract\n\n")
	w.writeString("| Status | Scenario | Benchmark | Metric | Baseline | Candidate | Delta | Budget |\n")
	w.writeString("| --- | --- | --- | --- | ---: | ---: | ---: | --- |\n")
	for _, row := range rows {
		w.printf(
			"| %s | %s | `%s` | `%s` | %.2f | %.2f | %.2f%% | %s |\n",
			row.Status,
			row.Scenario,
			row.Benchmark,
			row.Metric,
			row.Base,
			row.Candidate,
			row.Delta,
			row.Budget,
		)
	}
}

func (w *reportWriter) k6Summaries(k6 []k6Summary) {
	if len(k6) == 0 {
		return
	}
	w.writeString("\n## k6 summaries\n\n")
	w.writeString("| File | req/s | P50 ms | P95 ms | P99 ms | failed % |\n")
	w.writeString("| --- | ---: | ---: | ---: | ---: | ---: |\n")
	for _, summary := range k6 {
		w.printf(
			"| `%s` | %.2f | %.2f | %.2f | %.2f | %.4f |\n",
			summary.File,
			summary.Rate,
			summary.MedianMS,
			summary.P95MS,
			summary.P99MS,
			summary.FailedPct,
		)
	}
}

func (w *reportWriter) writeString(value string) {
	if _, err := w.builder.WriteString(value); err != nil {
		fatalf("write markdown report: %v", err)
	}
}

func (w *reportWriter) printf(format string, args ...any) {
	if _, err := fmt.Fprintf(&w.builder, format, args...); err != nil {
		fatalf("write markdown report: %v", err)
	}
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}
