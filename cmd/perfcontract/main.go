// Package main checks and reports SPACK performance contract data.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	budgetPath := flag.String("budget", filepath.Join("performance", "budgets.json"), "performance budget file")
	candidatePath := flag.String("candidate", "", "candidate Go benchmark output")
	baselinePath := flag.String("baseline", "", "baseline Go benchmark output")
	reportPath := flag.String("out", "", "markdown report output")
	metadataPath := flag.String("metadata", "", "metadata JSON output")
	k6Dir := flag.String("k6-dir", "", "directory containing k6 summary JSON files")
	flag.Parse()

	if *candidatePath == "" {
		fatalf("-candidate is required")
	}

	budgets := readBudgets(*budgetPath)
	candidate := readBenchResult(*candidatePath)
	baseline, hasBaseline := optionalBenchResult(*baselinePath)
	rows, failed := checkGoBenchmarks(budgets.GoBenchmarks, baseline, hasBaseline, candidate)
	k6 := readK6Summaries(*k6Dir)
	meta := collectMetadata()

	report := renderReport(budgets, rows, k6, meta, hasBaseline)
	writeReport(*reportPath, report)
	if *metadataPath != "" {
		writeJSON(*metadataPath, meta)
	}
	if failed {
		os.Exit(1)
	}
}

func writeReport(path, report string) {
	if path != "" {
		writeFile(path, []byte(report))
		return
	}
	if _, err := fmt.Print(report); err != nil {
		fatalf("write report to stdout: %v", err)
	}
}

func fatalf(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if _, err := os.Stderr.WriteString(message + "\n"); err != nil {
		panic(err)
	}
	os.Exit(1)
}
