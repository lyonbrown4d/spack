# SPACK Performance Contract

SPACK treats performance as a release artifact. Benchmarks are useful only when they are tied to a repeatable workload, a machine profile, raw data, and an explicit regression budget.

## Scenario matrix

| Scenario | Required measurements | Primary source |
| --- | --- | --- |
| Small hot-cache files | QPS, P50/P95/P99, `allocs/op` | Go HTTP benchmark, k6 static assets |
| Large files and Range requests | Throughput, peak memory, CPU | Go HTTP benchmark, release k6 workload |
| Brotli, Zstd, and Gzip hits | Hot-path latency, disk reads avoided | Resolver benchmarks, release k6 workload |
| First dynamic generation | First request latency, queue length, dropped tasks | Pipeline benchmark, runtime metrics |
| Image variants | Decode, resize, encode time, peak RSS | Resolver benchmark, release profile |
| Large directory startup | Source scan time, prepared snapshot time, memory usage | Startup benchmark, release report |
| Concurrency ladder | 1, 8, 64, 256, 1024 concurrent clients | k6 fixed workloads |
| Container limits | 0.25 CPU, 0.5 CPU, 64 MiB, 128 MiB | release k6 workload under Docker limits |

## CI policy

Pull requests:

- Run microbenchmarks for request path cleaning, resolver, memory cache, HTTP route, and pipeline enqueue.
- Generate a performance report artifact.
- Do not run full k6 unless the change touches performance-sensitive runtime paths.

`main`:

- Run the same fixed microbenchmark workload.
- Compare against the checked-in baseline when `performance/baselines/current/go-bench.txt` exists.
- Upload raw benchmark output, machine metadata, and the generated report.

Releases:

- Run the microbenchmark contract.
- Run the k6 frontend and split static-asset workloads.
- Attach raw JSON, Go benchmark output, machine metadata, and markdown report to the GitHub Release.

## Budgets

Budgets are stored in `performance/budgets.json`.

Core release budgets:

- Resolver benchmark `allocs/op` must not regress by more than 10%.
- Hot HTTP route `ns/op` must not regress by more than 8%.
- Pipeline enqueue `allocs/op` must not regress by more than 10%.
- k6 hot-cache P99 must not regress by more than 8%.
- The 64 MiB container workload must not OOM.
- 100k-asset startup must stay below the configured threshold in the budget file.

If a benchmark has no baseline yet, the report records candidate measurements and skips relative regression checks. The first stable release should publish `performance/baselines/vX.Y.Z/` and update `performance/baselines/current/`.

## Raw data requirements

Every published performance report should preserve:

- Git commit and tag.
- Go version, OS, architecture, CPU count, hostname, and timestamp.
- SPACK image digest and benchmark image digests.
- SPACK config used for the run.
- Raw Go benchmark output.
- Raw k6 summary JSON.
- Generated markdown report.

Do not edit raw benchmark output by hand.
