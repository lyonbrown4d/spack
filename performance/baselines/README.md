# Performance baselines

Store versioned raw baseline data here.

Recommended layout:

```text
performance/baselines/
  v0.1.0/
    go-bench.txt
    metadata.json
    k6/
      spack.json
      spack-frontend.json
  current/
    go-bench.txt
```

`performance/baselines/current/go-bench.txt` is the file used by `task perf:contract:main` and the performance workflow for regression checks.

Do not commit generated data unless it represents an intentional release baseline.
