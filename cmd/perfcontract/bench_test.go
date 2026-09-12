package main

import (
	"reflect"
	"strings"
	"testing"
)

type benchResultTestCase struct {
	name  string
	lines []string
	want  benchResult
}

func TestParseBenchResult(t *testing.T) {
	t.Parallel()

	for _, test := range benchResultTestCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := parseBenchResult([]byte(strings.Join(test.lines, "\n")))
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("parseBenchResult() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func benchResultTestCases() []benchResultTestCase {
	return []benchResultTestCase{
		subBenchmarkResultTestCase(),
		unitConversionResultTestCase(),
		legalAndIllegalResultTestCase(),
		originalPrecisionResultTestCase(),
		nameWithoutCPUSuffixResultTestCase(),
	}
}

func subBenchmarkResultTestCase() benchResultTestCase {
	return benchResultTestCase{
		name: "sub benchmark and configuration",
		lines: []string{
			"goos: linux",
			"goarch: amd64",
			"pkg: example.com/project",
			"cpu: Example CPU",
			"BenchmarkParse/small-8 100 100 ns/op 64 B/op 2 allocs/op",
			"BenchmarkParse/small-16 100 300 ns/op 128 B/op 4 allocs/op",
		},
		want: benchResult{
			"BenchmarkParse/small": {
				"ns_per_op":     200,
				"b_per_op":      96,
				"allocs_per_op": 3,
			},
		},
	}
}

func unitConversionResultTestCase() benchResultTestCase {
	return benchResultTestCase{
		name: "unit conversion",
		lines: []string{
			"BenchmarkUnits-4 10 0.000002 sec/op 0.5 MB/op 3 allocs/op",
		},
		want: benchResult{
			"BenchmarkUnits": {
				"ns_per_op":     2000,
				"b_per_op":      500000,
				"allocs_per_op": 3,
			},
		},
	}
}

func legalAndIllegalResultTestCase() benchResultTestCase {
	return benchResultTestCase{
		name: "legal and illegal lines",
		lines: []string{
			"ordinary test output",
			"BenchmarkStartedOnly",
			"BenchmarkBadIterations nope 42 ns/op",
			"BenchmarkMissingUnit 1 42",
			"BenchmarkUnknownMetric 1 9 widgets/op",
			"BenchmarkValid 1 42 ns/op",
			"BenchmarkAlsoValid-2 1 2 B/op",
		},
		want: benchResult{
			"BenchmarkValid": {
				"ns_per_op": 42,
			},
			"BenchmarkAlsoValid": {
				"b_per_op": 2,
			},
		},
	}
}

func originalPrecisionResultTestCase() benchResultTestCase {
	return benchResultTestCase{
		name: "original metric precision",
		lines: []string{
			"BenchmarkPrecision-8 1 42 ns/op 0.1 B/op 0.2 allocs/op",
		},
		want: benchResult{
			"BenchmarkPrecision": {
				"ns_per_op":     42,
				"b_per_op":      0.1,
				"allocs_per_op": 0.2,
			},
		},
	}
}

func nameWithoutCPUSuffixResultTestCase() benchResultTestCase {
	return benchResultTestCase{
		name: "name without CPU suffix",
		lines: []string{
			"BenchmarkHyphenated-name 1 7 ns/op",
		},
		want: benchResult{
			"BenchmarkHyphenated-name": {
				"ns_per_op": 7,
			},
		},
	}
}
