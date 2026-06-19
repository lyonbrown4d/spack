package main

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
)

func readK6Summaries(dir string) []k6Summary {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	entries, ok := readRepoDir(dir)
	if !ok {
		return nil
	}

	summaries := make([]k6Summary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if summary, ok := parseK6Summary(filepath.Join(dir, entry.Name())); ok {
			summaries = append(summaries, summary)
		}
	}
	slices.SortFunc(summaries, func(left, right k6Summary) int {
		return strings.Compare(left.File, right.File)
	})
	return summaries
}

func parseK6Summary(path string) (k6Summary, bool) {
	var raw struct {
		Metrics map[string]struct {
			Values map[string]float64 `json:"values"`
		} `json:"metrics"`
	}
	if err := json.Unmarshal(readRepoFile(path), &raw); err != nil {
		return k6Summary{}, false
	}
	duration := raw.Metrics["http_req_duration"].Values
	reqs := raw.Metrics["http_reqs"].Values
	failed := raw.Metrics["http_req_failed"].Values
	return k6Summary{
		File:      filepath.Base(path),
		Rate:      reqs["rate"],
		MedianMS:  duration["med"],
		P95MS:     duration["p(95)"],
		P99MS:     duration["p(99)"],
		FailedPct: failed["rate"] * 100,
	}, true
}
